package main

import(
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
	"sync"
	"strings"
	"math/rand"
)

//Numero de processos
const N = 3
//Tamanho do terreno
const LENGTH = 10
const TEMP_MAX = 45
const TEMP_MIN = 35
const TEMP_THRESHOLD = 40
//porta de referencia
const refport = 4567
const RECUE = "R"
const AVANCE = "A"
const ELECTION = "E"
const OK = "OK"
const LEADER = "L"
const QUESTION = "Q"
//Valores de timeout
const ELECTION_TIMEOUT = 100
const LEADER_TIMEOUT = 1500
const MOVE_DELAY = 500

type Process struct{
	id int 	//id do processo
	conns []*net.UDPConn  //array com conexoes para os outros procs
	ports []string  //array com portas dos outros procs
	ServerConn *net.UDPConn  //a sua conexao servidora
	x int  //coordenadas atuais
	y int
	sucX int  //posicoes sucessoras na movimentacao
	sucY int
	terrainMarks []bool  //marca posicoes ja verificadas no terreno
	leader int  //Identifica o lider
	isLeader bool  //identifica se eh lider
	sensor []int  //mantem cópia do mapa de temperaturas do terreno
	electionOngoing bool  //eleição acontecendo
	awaitingPermission bool  //permissão pra andar
	dontTimeout chan string  //timeout de morte do lider
	alive bool  //está vivo
}

//mapeia coordenadas do terreno em indices para o vetor
func coord2ind(x, y int) (int){
	return y*LENGTH + x
}

//gera os valores de temperatura de cada ponto do terreno
func createTerrain(terrain []int){
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i:=0; i < LENGTH*LENGTH -1;i++{
		temp:= r.Intn(10)+TEMP_MIN
		fmt.Println("T = ",temp)
		terrain[i] = temp
	}
	//Fazer os processos iniciarem em areas seguras
	for i:= 0; i < N; i++{
		coord:=LENGTH/N/2+LENGTH/N*i
		terrain[coord2ind(coord,coord)] = TEMP_MIN
	}
}

//Nao precisa se preocupar com o funcionamento desta funcao
func (p *Process) MakeConnections(){
	for j:=0; j < N; j++{
		//Estabelecendo as conexoes
		p.ports[j] = strconv.Itoa(refport+j)

		ServerAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:"+p.ports[j])
		checkError(err)

		//Pede porta disponivel ao sistema operacional
		LocalAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
		checkError(err)

		p.conns[j], err = net.DialUDP("udp", LocalAddr, ServerAddr)
		checkError(err)
	}
	//Selecionando a porta de cada processo
	port := strconv.Itoa(refport+p.id)
	port = ":"+port
	ServerAddr, err := net.ResolveUDPAddr("udp", port)
	checkError(err)

	//Fazendo com os processos esperem por dados
	p.ServerConn, err = net.ListenUDP("udp", ServerAddr)
	checkError(err)
}

//Envia broadcast de alguma mensagem a todos os outros processos
func (p *Process) sendBroad(msg string) {
	t:=time.Now()
	from:= p.id
	fmt.Println("<",t.Format("15:04:05.000000"), " Process:", p.id, " ; Sending  '", msg ,"' to ALL >")
	for j:=0; j < N; j++{
		if j != p.id {
			to := j
			msg = strconv.Itoa(from)+";"+strconv.Itoa(to)+";"+msg
			buf := []byte(msg)
			_, err := p.conns[j].Write(buf)
			checkError(err)	
		}
	}
}

func (p *Process) doElection(amLeader bool) {
	if p.electionOngoing {
		return;
	}
	p.electionOngoing = true;
	p.isLeader = amLeader;
	p.leader = -1;

	//Declara início de eleição
	for j:=0; j<N; j++ {
		if j != p.id {
			p.sendTo(ELECTION, j);
		}
	}
	time.Sleep(time.Duration((p.id+1) * ELECTION_TIMEOUT) * time.Millisecond);
	if p.isLeader {
		fmt.Println(p.id, "elected leader");
		p.leader = p.id;
		for j:=0; j<N; j++ {
			if j != p.id {
				p.sendTo(LEADER, j);
			}
		}
	}
	p.electionOngoing = false;
}

//Eh realizado um loop infinito que fica aguardando por chegada de msgs
//Pode ser interessante implementar um timeout
//Todas as msgs estao no formato <1>;<2>;<3>;<4>(opcional)
//<1> PID de origem
//<2> PID de destino
//<3> tipo da mensagem
//<4> opcional, no caso de Q, envia o valor da temperatura
func (p *Process) runProcess() {
	p.terrainMarks[coord2ind(p.x,p.y)] = true;

	//Conecta e aguarda os outros conectarem
	p.MakeConnections();
	time.Sleep(100 * time.Millisecond);

	//Morte
	go p.scriptedDeath(5000 * (p.id+1));

	//Eleição inicial
	go p.doElection(true);

	buf := make([]byte, 100);
	for p.alive {
		//Se não estiver esperando pra se mover, enviar nova solicitação de movimento
		if !p.awaitingPermission {
			go p.getSucXY();
		}

		n, _, err := p.ServerConn.ReadFromUDP(buf);
		checkError(err);
		msg := string(buf[0:n]);
		data := strings.Split(msg,";");
		from, _ := strconv.Atoi(data[0]);
		fmt.Println("from: ", data[0], "  to: ", data[1], "content: ", data[2]);
		t := time.Now();
		fmt.Println("<", t.Format("15:04:05.000000"), " Process:",p.id," ; Received '",msg, "' >");
		

		switch msgType := data[2]; msgType {
			//Caso seja uma questão
			case QUESTION:
				var replymsg string;
				temp,_:= strconv.Atoi(data[3]);
				if temp < TEMP_THRESHOLD {
					replymsg = AVANCE;
				} else {
					replymsg = RECUE;
				}
				p.sendTo(replymsg, from);

			//Caso seja o caso, o robo fica parado na sua posicao e marca como
			//visitada a posicao que perguntou
			case RECUE:
				p.terrainMarks[coord2ind(p.sucX, p.sucY)] = true
				p.awaitingPermission = false;
				p.dontTimeout <- "message received"; //avisa que o líder não deu timeout

			//A posicao do robo somente eh alterada se for possivel se locomover
			case AVANCE:
				p.terrainMarks[coord2ind(p.sucX, p.sucY)] = true
				p.x = p.sucX;
				p.y = p.sucY;
				p.awaitingPermission = false;
				p.dontTimeout <- "message received"; //avisa que o líder não deu timeout

			//Mensagem de eleição
			case ELECTION:
				if (from < p.id) {
					go p.doElection(false);
					p.isLeader = false;
				} else {
					p.sendTo(ELECTION, from);
					go p.doElection(true);
				}
			
			//Novo lider declarado
			case LEADER:
				p.leader = from;
				p.isLeader = (from == p.id);

			//Mensagem desconhecida
			default:
				fmt.Println("Unknown message received:", msg);
		}
	}
	
	fmt.Println("Process", strconv.Itoa(p.id), "died");
}

//Envia msg para processo alvo
func (p *Process) sendTo(msg string, id int) {
	t:=time.Now()
	//fmt.Println("<",t.Format("15:04:05.000000"), " Process:", p.id, " ; Sending '", msg ,"' >")
	from := p.id
	to := strconv.Itoa(id)
	msg = strconv.Itoa(from)+";"+to+";"+msg
	fmt.Println("<",t.Format("15:04:05.000000"), " Process:", p.id, " ; Sending '", msg ,"' >")
	buf := []byte(msg)
	_, err := p.conns[id].Write(buf)
	checkError(err)
}

func checkError(err error){
	if err != nil {
		printErr(err)
		os.Exit(0)
	}
}

func printErr(err error){
	fmt.Println("< Server; Error: ",err, " >")
}


//Faz o robo se movimentar
func (p *Process) getSucXY() {
	if p.awaitingPermission || !p.alive{
		return;
	}
	p.awaitingPermission = true;

	//Delay para vizualização
	time.Sleep(time.Duration(MOVE_DELAY) * time.Millisecond);

	r := rand.New(rand.NewSource(time.Now().UnixNano()));
	//gera uma lista aleatoria para proximo caminho com todos os vizinhos
	sucessores := r.Perm(9);
	var auxY, auxX int;
	for i:=0; i < 9; i++ {
		auxX = sucessores[i]%3-1;
		auxY = sucessores[i]/3-1;
		p.sucX = auxX + p.x;
		p.sucY = auxY + p.y;
		//fmt.Println("processo ",p.id,"  sucX: ",p.sucX,"   sucY: ", p.sucY, "   ind: ", coord2ind(p.sucX, p.sucY))
		if p.sucY >= 0 && p.sucX >= 0 && p.sucX < LENGTH && p.sucY < LENGTH && p.terrainMarks[coord2ind(p.sucX, p.sucY)] == false {
			break;
		}
	}
	if !(p.sucY >= 0 && p.sucX >= 0 && p.sucX < LENGTH && p.sucY < LENGTH && p.terrainMarks[coord2ind(p.sucX, p.sucY)] == false) {
		for i:=0; i < 9; i++ {
			auxX = sucessores[i]%3-1;
			auxY = sucessores[i]/3-1;
			p.sucX = auxX + p.x;
			p.sucY = auxY + p.y;
			if p.sucY >= 0 && p.sucX >= 0 && p.sucX < LENGTH && p.sucY < LENGTH {
				break;
			}
		}
	}
	tempStr := strconv.Itoa(p.sensor[coord2ind(p.sucX, p.sucY)]);
	//Espera lider ser eleito
	for p.leader == -1 { }
	p.sendTo(QUESTION+";"+tempStr,p.leader);

	//Verifica timeout do lider
	select {
		case <- p.dontTimeout:
		case <- time.After(time.Millisecond * LEADER_TIMEOUT):
			fmt.Println("Leader timeout");
			go p.doElection(!p.isLeader);
			p.awaitingPermission = false;
	}
}

func (p *Process) scriptedDeath(timeout int) {
	time.Sleep(time.Duration(timeout) * time.Millisecond);
	p.alive = false;
}


//Neste exemplo, o líder não é escolhido por meio de processo de eleição
//Alem disto, nao se movimenta tambem
//Cada um dos outros robos, envia uma msg e aguarda resposta
//O lider apenas ouve as perguntas e responde 
func main(){
	//Numero de processos
	var wg sync.WaitGroup;
	//Gerando matriz do terreno com temperaturas
	terrain := make([]int, LENGTH*LENGTH);
	createTerrain(terrain);


	procs := make([]*Process, N);
	for id := 0; id < N; id++{
		wg.Add(1)
		go func(i int){
			//Inicializando cada um dos processos
			procs[i] = &Process {
				id: i, 
				conns: make([]*net.UDPConn, N),
				ports:make([]string, N),
				y: LENGTH/N/2+LENGTH/N*i,  //posicoes 'ok'
				x: LENGTH/N/2+LENGTH/N*i,
				terrainMarks: make([]bool, LENGTH*LENGTH),
				electionOngoing: false,
				isLeader: false,
				leader: -1,
				dontTimeout: make(chan string),
				sensor: terrain,
				alive: true };
			p := procs[i]
			p.runProcess();
		}(id)
	}
	wg.Wait()
}