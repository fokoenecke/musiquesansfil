package main

import (
	"flag"
	"fmt"
	"math/rand"
	"time"

	"github.com/hypebeast/go-osc/osc"
)

var serverHost = flag.String("s", "192.168.178.20", "Set Pure Data server host")
var serverPort = flag.Int("p", 9001, "Set Pure Data server port")

func main() {

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
