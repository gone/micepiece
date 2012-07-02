package main

import (
	"code.google.com/p/go.net/websocket"
	"encoding/json"
	"fmt"
	"net/http"
	"text/template"
	"time"
	"math/rand"
)

const (
	MAX_PER_HUB int = 200
)


type jsonRPC func(*connection, interface{})

type PlayerData struct {
	X int
	Y int
	Color string
}

type message struct {
	Event string
	Data  interface{}
}

type connection struct {
	wsock *websocket.Conn
	send  chan []byte
	hub *hub
}

type hub struct {
	connections map[*connection]PlayerData

	input chan string

	register chan *connection

	unregister chan *connection
}

var COLORS [3]string = [3]string{"#FF0000", "#00FF00", "#0000FF"}

var hubs []*hub = make([]*hub, 0)

func makeHub() *hub{
	h := hub{
		connections: make(map[*connection]PlayerData),
		input:       make(chan string),
		unregister:  make(chan *connection),
		register:    make(chan *connection),
	}
	go h.run()
	return &h
}
func (h *hub) updateClients() {
	coords := make([]PlayerData, 0, len(h.connections))
	for _, v := range h.connections {
		coords = append(coords, v)
	}
	json_message, err := json.Marshal(message{"update", coords})
	if err != nil {
		fmt.Println("update")
		fmt.Println(err)
	}
	for c := range h.connections {
		c.send <- json_message
	}
}

func (h *hub) run() {
	timeout := make(chan bool, 1)
	go func() {
		for {
			sleep_time := time.Duration(10000000)
			time.Sleep(sleep_time * time.Nanosecond)
			timeout <- true
		}
	}()
	newround := make(chan bool, 1)
	go func() {
		for {
			sleep_time := time.Duration(5)
			time.Sleep(sleep_time * time.Second)
			newround <- true
		}
	}()
	for {
		select {
		case c := <-h.register:
			h.connections[c] = PlayerData{X: 0, Y: 0}

		case c := <-h.unregister:
			delete(h.connections, c)

		case <-timeout:
			go h.updateClients()

		case <-newround:
			go h.newRound()
		}
	}
}

func (h *hub) update(c *connection, coords PlayerData) {
	h.connections[c] = coords
}

func (h *hub) newRound(){
 	var points = make([]PlayerData, 0)
 	for i := 0; i < 11; i++ {
		x, y := rand.Intn(1000),  rand.Intn(1000)
		c  := rand.Intn(len(COLORS))
		points = append(points, PlayerData{X:x, Y:y, Color:COLORS[c]})
 	}
	json_message, err := json.Marshal(message{"newRound", points})
	if err != nil {
		fmt.Println("newRound")
		fmt.Println(err)
	}
	for c := range h.connections {
		c.send <- json_message
	}
}


func (h *hub) removeTriangle(x, y int){
	triangle_to_remove := PlayerData{X:x, Y:y}
	json_message, err := json.Marshal(message{"removeTriangle", triangle_to_remove})
	if err != nil {
		fmt.Println("removeTriangle")
		fmt.Println(err)
	}
	for c := range h.connections {
		c.send <- json_message
	}
}

func mouseMove(c *connection, data interface{}) {
	d := data.(map[string]interface{})
	x, _ := d["x"].(float64)
	y, _ := d["y"].(float64)
	color, _ := d["color"].(string)
	c.hub.update(c, PlayerData{
		X: int(x),
		Y: int(y),
		Color: color,
	})
}

func removeTriangle(c *connection, data interface{}) {
	d := data.(map[string]interface{})
	x, _ := d["x"].(float64)
	y, _ := d["y"].(float64)
		c.hub.removeTriangle(int(x), int(y))
}


var events = map[string]jsonRPC{
	"mousemove": mouseMove,
	"removeTriangle": removeTriangle,
}



func (c *connection) reader() {

	for {
		var m message
		err := websocket.JSON.Receive(c.wsock, &m)
		if err != nil {
			fmt.Println("reader")
			fmt.Println(err)
			break
		}
		go events[m.Event](c, m.Data)
	}
	c.wsock.Close()
}

func (c *connection) writer() {
	for message := range c.send {
		err := websocket.Message.Send(c.wsock, string(message))
		if err != nil {
			fmt.Println("writer")
			fmt.Println(err)
			break
		}
	}
	c.wsock.Close()
}

func Handler(wsock *websocket.Conn) {
	c := &connection{
		send:  make(chan []byte, 256),
		wsock: wsock,
	}
	//loop through hubs until you find one w/ less than max_per_hub
	var thehub *hub
	for _, h := range hubs {
		// this is a race condition - could try to put connecton on hub at the same time
		// as another handler is doing so. probably need to mutex the channel send/recv? How do I do that?
		if len(h.connections) < MAX_PER_HUB {
			thehub = h
		}
	}
	//all full
	if thehub == nil {
		thehub = makeHub()
		hubs = append(hubs, thehub)
	}
	thehub.register <- c
	c.hub = thehub
	defer func(){thehub.unregister <- c}()
	go c.writer()
	c.reader()

}

func homeHandler(c http.ResponseWriter, req *http.Request) {
	homeTemplate, _ := template.ParseFiles("home.html")
	homeTemplate.Execute(c, req.Host)

}

func main() {
	var now time.Time = time.Now()
	rand.Seed(now.Unix())

	http.HandleFunc("/", homeHandler)
	http.Handle("/ws", websocket.Handler(Handler))
	http.ListenAndServe("127.0.0.1:8080", nil)
}
