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



type Process struct{
	id int 	//id do processo
	conns []*net.UDPConn  //array com conexoes para os outros procs
	ports []string  //array com portas dos outros procs
	ServerConn *net.UDPConn  //a sua conexao servidora
	x int  //coordenadas atuais
	y int
	sucX int
	sucY int
	terrainMarks []bool
	leader int
	isLeader bool
	sensor []int
}

func coord2ind(x, y int) (int){
	return y*LENGTH + x
}

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

func (p *Process) listen() (int){
	buf := make([]byte, 100)
	for{
		n, _, err := p.ServerConn.ReadFromUDP(buf)
		checkError(err)
		msg := string(buf[0:n])
		data := strings.Split(msg,";")
		from := data[0]
		fmt.Println("from: ",data[0],"  to: ", data[1],  "content: ", data[2])
		t:=time.Now()
		fmt.Println("<", t.Format("15:04:05.000000"), " Process:",p.id," ; Received '",msg, "' >")
		
		msgType := data[2]
		if msgType == QUESTION{
			var replymsg string
			fromInt,_ := strconv.Atoi(from)
			temp,_:= strconv.Atoi(data[3])
			if temp < TEMP_THRESHOLD{
				replymsg = AVANCE
			}else{
				replymsg = RECUE
			}
			p.sendTo(replymsg, fromInt)
		}
		//Caso seja o caso, o robo fica parado na sua posicao e marca como
		//visitada a posicao que perguntou
		if msgType == RECUE{
			p.terrainMarks[coord2ind(p.sucX, p.sucY)] = true
		}
		if msgType == AVANCE{
			p.terrainMarks[coord2ind(p.sucX, p.sucY)] = true
			p.x = p.sucX;
			p.y = p.sucY;
		}
		//Endereco na mensagem em ascii
		aux,_ := strconv.Atoi(data[0])
		return aux
	}
}

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

func (p *Process) move(){
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	//gera uma lista aleatoria para proximo caminho com todos os vizinhos
	sucessores := r.Perm(9)
	var auxY, auxX int
	for i:=0; i < 9; i++{
		auxX = sucessores[i]%3-1
		auxY = sucessores[i]/3-1
		p.sucX = auxX + p.x
		p.sucY = auxY + p.y
		//fmt.Println("processo ",p.id,"  sucX: ",p.sucX,"   sucY: ", p.sucY, "   ind: ", coord2ind(p.sucX, p.sucY))
		if p.sucY >= 0 && p.sucX >= 0 && p.sucX < LENGTH && p.sucY < LENGTH && p.terrainMarks[coord2ind(p.sucX, p.sucY)] == false{
			break
		}
	}
	if p.sucY >= 0 && p.sucX >= 0 && p.sucX < LENGTH && p.sucY < LENGTH && p.terrainMarks[coord2ind(p.sucX, p.sucY)] == false{
		tempStr := strconv.Itoa(p.sensor[coord2ind(p.sucX, p.sucY)])
		p.sendTo(QUESTION+";"+tempStr,p.leader)	
	}
}


func main(){
	//Numero de processos
	var wg sync.WaitGroup
	//Gerando matriz do terreno com temperaturas
	terrain := make([]int, LENGTH*LENGTH)
	createTerrain(terrain)

	procs := make([]*Process, N)
	for id := 0; id < N; id++{
		wg.Add(1)
		go func(i int){
			//Inicializando cada um dos processos
			procs[i] = &Process{
				id: i, 
				conns: make([]*net.UDPConn, N),
				ports:make([]string, N),
				y: LENGTH/N/2+LENGTH/N*i,
				x: LENGTH/N/2+LENGTH/N*i,
				terrainMarks: make([]bool, LENGTH*LENGTH),
				isLeader: false,
				sensor: terrain}
			p := procs[i]
			p.terrainMarks[coord2ind(p.x,p.y)] = true
			fmt.Println(p.id,": x = ",p.x)
			p.MakeConnections()

			//A principio o lider sera hardcoded
			if p.id == 0 {
				p.isLeader = true
				//Enviando broadcast
				msg:="Hello broad, from Process "
				p.sendBroad(msg)

				//Esperando pelas respostas dos outros processos
				for k:=true;k==true;{
					p.listen()	
				}
			} else {
				p.leader = 0
				senderID := p.listen()

				//Enviando resposta
				msg:="Hi, from Process "+strconv.Itoa(p.id)+"!"
				p.sendTo(msg, senderID)
				for{
					//Esperando pela chegada de alguma mensagem (broadcast)
					//senderID := p.listen()

					//Enviando resposta
					//msg:="Hi, from Process "+strconv.Itoa(p.id)+"!"
					//p.sendTo(msg, senderID)
					p.move()
					p.listen()
				}
			}

		}(id)
	}
	wg.Wait()
}