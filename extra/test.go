package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"text/template"
	"time"

	"github.com/googollee/go-socket.io"
	"github.com/hypebeast/go-osc/osc"
)

var serverHost = flag.String("s", "192.168.178.20", "Set Pure Data server host")
var serverPort = flag.Int("p", 9001, "Set Pure Data server port")

type data struct{}

func hello(w http.ResponseWriter, r *http.Request) {
	t, _ := template.ParseFiles("../tmpl/index.html")

	val := data{}
	t.Execute(w, val)
}

func main() {

	server, err := socketio.NewServer(nil)
	if err != nil {
		log.Fatal(err)
	}
	server.On("connection", func(so socketio.Socket) {
		log.Println("on connection")
		so.Join("chat")
		so.On("chat message", func(msg string) {
			fmt.Println(so, msg)
			log.Println("emit:", so.Emit("chat message", msg))
			so.BroadcastTo("chat", "chat message", msg)
		})
		so.On("disconnection", func() {
			log.Println("on disconnect")
		})
	})
	server.On("error", func(so socketio.Socket, err error) {
		log.Println("error:", err)
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/", hello)
	mux.Handle("/socket.io/", server)
	log.Println("Serving at localhost:5000...")
	log.Fatal(http.ListenAndServe(":5000", mux))

}

func main2() {

	client := osc.NewClient(*serverHost, *serverPort)
	fmt.Println("server:", *serverHost, "port:", *serverPort)

	c := 0

	for {

		sendMessage(client, ((c+rand.Intn(4))%4)+1, "kick")
		sendMessage(client, ((c+rand.Intn(4))%4)+1, "snare")
		sendMessage(client, ((c+rand.Intn(4))%4)+1, "bass")
		sendMessage(client, ((c+rand.Intn(4))%4)+1, "hh")
		dur := time.Duration(rand.Intn(5)) * time.Second
		fmt.Println(dur)
		time.Sleep(dur)
		c++

	}

}

func sendMessage(client *osc.Client, level int, instrument string) {
	fmt.Println("sending:", level, "to:", instrument)
	msg := osc.NewMessage("/instrument/" + instrument)
	msg.Append(int32(level))
	client.Send(msg)
}
