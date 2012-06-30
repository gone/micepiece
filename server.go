package main

import (
	"code.google.com/p/go.net/websocket"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"text/template"
	"time"
)

type jsonRPC func(*connection, interface{})

type coord struct {
	X int
	Y int
}

type message struct {
	Event string
	Data  interface{}
}

type connection struct {
	wsock *websocket.Conn
	send  chan []byte
}

type hub struct {
	connections map[*connection]coord

	input chan string

	register chan *connection

	unregister chan *connection
}

var h = hub{
	connections: make(map[*connection]coord),
	input:       make(chan string),
	unregister:  make(chan *connection),
	register:    make(chan *connection),
}

func (h *hub) updateClients() {
	coords := make([]coord, 0, len(h.connections))
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
	for {
		select {
		case c := <-h.register:
			h.connections[c] = coord{X: 0, Y: 0}

		case c := <-h.unregister:
			delete(h.connections, c)

		case <-timeout:
			go h.updateClients()
		}
	}
}

func (h *hub) update(c *connection, coords coord) {
	h.connections[c] = coords
}

func mousemove(c *connection, data interface{}) {
	d := data.(map[string]interface{})
	x, _ := d["x"].(int)
	y, _ := d["y"].(int)
	h.update(c, coord{X: x, Y: y})

}

var events = map[string]jsonRPC{"mousemove": mousemove}

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

func parseMessage(message string) coord {
	s := strings.Split(message, ":")
	x, _ := strconv.Atoi(s[0])
	y, _ := strconv.Atoi(s[1])
	return coord{X: x, Y: y}
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
	h.register <- c
	defer func() { h.unregister <- c }()
	go c.writer()
	c.reader()

}

func homeHandler(c http.ResponseWriter, req *http.Request) {
	homeTemplate, _ := template.ParseFiles("home.html")
	homeTemplate.Execute(c, req.Host)

}

func main() {
	go h.run()
	http.HandleFunc("/", homeHandler)
	http.Handle("/ws", websocket.Handler(Handler))
	http.ListenAndServe("127.0.0.1:8080", nil)
}
