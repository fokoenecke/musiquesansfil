package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"sort"
	"sync"
	"text/template"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/googollee/go-socket.io"
	"github.com/hypebeast/go-osc/osc"
)

var serverHost = flag.String("s", "127.0.0.1", "Set Pure Data server host")
var serverPort = flag.Int("p", 9001, "Set Pure Data server port")
var device = flag.String("d", "wlan0", "Set device to listen on")

type activity struct {
	packets      int
	sizeSum      int
	since        time.Time
	instrument   int
	currentLevel int
}

func (activity *activity) increment() {
	activity.packets++
}

func (activity *activity) currentPackets() int {
	return activity.packets
}

func (activity *activity) addPacketSize(size int) {
	activity.sizeSum += size
}

type instrument struct {
	name               string
	mapLevel           func(bps float64) int
	adjustCurrentLevel func(client *activity, targetLevel int)
	sendMessage        func(client *osc.Client, level int, speed int, instrument string)
}

func mapSpeedLevel(pps float64) int {
	var level int
	if pps > 6 {
		level = 4
	} else if pps > 3 {
		level = 3
	} else if pps > 2 {
		level = 2
	} else if pps > 1 {
		level = 1
	} else if pps > .5 {
		level = 8
	} else {
		level = 16
	}
	return level
}

func mapDrumLevel(bps float64) int {
	var level int
	if bps > 150 {
		level = 4
	} else if bps > 120 {
		level = 3
	} else if bps > 70 {
		level = 2
	} else if bps > 30 {
		level = 1
	} else if bps > 15 {
		level = 8
	} else if bps > 5 {
		level = 16
	} else {
		level = 0
	}
	return level
}

func mapMelodyLevel(bps float64) int {
	var level int
	if bps > 150 {
		level = 8
	} else if bps > 130 {
		level = 7
	} else if bps > 100 {
		level = 6
	} else if bps > 80 {
		level = 5
	} else if bps > 40 {
		level = 4
	} else if bps > 30 {
		level = 3
	} else if bps > 10 {
		level = 2
	} else if bps > 5 {
		level = 1
	} else {
		level = 0
	}
	return level
}

func adjustDrumLevel(client *activity, targetLevel int) {
	if targetLevel > 4 {
		client.currentLevel = targetLevel
	} else {
		if client.currentLevel > 4 {
			client.currentLevel = 1
		}
		adjustLevel(client, targetLevel)
	}
}

func adjustMelodyLevel(client *activity, targetLevel int) {
	client.currentLevel = targetLevel
}

func adjustLevel(client *activity, targetLevel int) {
	if client.currentLevel < targetLevel {
		client.currentLevel++
	} else if client.currentLevel > targetLevel {
		client.currentLevel--
	}
}

type data struct{}

func hello(w http.ResponseWriter, r *http.Request) {
	t, _ := template.ParseFiles("tmpl/index.html")

	val := data{}
	t.Execute(w, val)
}

func serv() *socketio.Server {

	server, err := socketio.NewServer(nil)
	if err != nil {
		log.Fatal(err)
	}
	server.On("connection", func(so socketio.Socket) {
		log.Println("on connection")
		so.Join("chat")
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
	go http.ListenAndServe(":8000", mux)

	return server
}

func main() {
	flag.Parse()

	devices, err := pcap.FindAllDevs()
	if err != nil {
		log.Fatal(err)
	}

	filter := ""
	for _, dev := range devices {
		if dev.Name == *device {
			fmt.Println(dev)
			for _, address := range dev.Addresses {
				filter = fmt.Sprintf("not ip host %s ", address.IP)
				break
			}
		}
	}

	handle, err := pcap.OpenLive(*device, 65536, true, 0)
	if err != nil {
		defer handle.Close()
		panic(err)
	}

	err = handle.SetBPFFilter(filter)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(handle.LinkType())
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	client := osc.NewClient(*serverHost, *serverPort)

	var clients = struct {
		sync.RWMutex
		m              map[string]*activity
		instrumentPool []int
	}{m: make(map[string]*activity)}

	instruments := map[int]*instrument{
		0: &instrument{"kick", mapDrumLevel, adjustDrumLevel, sendDrumMessage},
		1: &instrument{"snare", mapDrumLevel, adjustDrumLevel, sendDrumMessage},
		2: &instrument{"hh", mapDrumLevel, adjustDrumLevel, sendDrumMessage},
		3: &instrument{"bass", mapMelodyLevel, adjustMelodyLevel, sendMelodyMessage},
		4: &instrument{"melody", mapMelodyLevel, adjustMelodyLevel, sendMelodyMessage},
	}

	server := serv()
	ticker := time.NewTicker(time.Second * 2)
	go func() {
		for t := range ticker.C {
			fmt.Println("Tick at", t)
			clients.Lock()
			for key, value := range clients.m {
				elapsed := time.Since(value.since)
				pps := float64(value.currentPackets()) / elapsed.Seconds()
				bps := float64(value.sizeSum) / elapsed.Seconds()

				instrument, ok := instruments[value.instrument]
				if ok {
					targetLevel := instrument.mapLevel(bps)
					if targetLevel == 0 {
						clients.instrumentPool = append(clients.instrumentPool, value.instrument)
						sort.Ints(clients.instrumentPool)
						delete(clients.m, key)
					}
					speed := mapSpeedLevel(pps)
					instrument.adjustCurrentLevel(value, targetLevel)
					instrument.sendMessage(client, value.currentLevel, speed, instrument.name)
				}
				fmt.Println("Key:", key, "instrument:", value.instrument, "elapsed:", elapsed, "Packets:", value.currentPackets(), "pps:", pps, "bps:", bps)
				server.BroadcastTo("chat", "chat message", key)
			}
			if len(clients.m) > 0 {
				fmt.Println()
			}

			clients.Unlock()
		}
	}()

	for packet := range packetSource.Packets() {
		fmt.Println(packet)
		// Let's see if the packet is an ethernet packet
		ethernetLayer := packet.Layer(layers.LayerTypeEthernet)
		if ethernetLayer != nil {
			fmt.Println("Ethernet layer detected.")
			ethernetPacket, _ := ethernetLayer.(*layers.Ethernet)
			fmt.Println("Source MAC: ", ethernetPacket.SrcMAC)
			fmt.Println("Destination MAC: ", ethernetPacket.DstMAC)
			// Ethernet type is typically IPv4 but could be ARP or other
			fmt.Println("Ethernet type: ", ethernetPacket.EthernetType)

			clients.Lock()
			_, ok := clients.m[ethernetPacket.SrcMAC.String()]
			packetLength := len(ethernetPacket.Payload)
			fmt.Println(clients.instrumentPool)
			if ok {
				p := clients.m[ethernetPacket.SrcMAC.String()]
				p.increment()
				p.addPacketSize(packetLength)
			} else {
				var instrument int
				if len(clients.instrumentPool) != 0 {
					instrument = clients.instrumentPool[0]
					clients.instrumentPool = append(clients.instrumentPool[:0], clients.instrumentPool[0+1:]...)
				} else {
					instrument = len(clients.m)
				}
				clients.m[ethernetPacket.SrcMAC.String()] = &activity{1, packetLength, time.Now(), instrument, 0}
			}
			_, ok = clients.m[ethernetPacket.DstMAC.String()]
			if ok {
				p := clients.m[ethernetPacket.DstMAC.String()]
				p.increment()
				p.addPacketSize(len(ethernetPacket.Contents))
			} else {
				var instrument int
				if len(clients.instrumentPool) != 0 {
					instrument = clients.instrumentPool[0]
					clients.instrumentPool = append(clients.instrumentPool[:0], clients.instrumentPool[0+1:]...)
				} else {
					instrument = len(clients.m)
				}
				clients.m[ethernetPacket.DstMAC.String()] = &activity{1, packetLength, time.Now(), instrument, 0}
			}
			clients.Unlock()

		}

		ip4Layer := packet.Layer(layers.LayerTypeIPv4)
		if ip4Layer != nil {
			//var ip = ip4Layer.(*layers.IPv4)
			//fmt.Println("Source IP: ", ip.SrcIP)
			//fmt.Println("Destination IP: ", ip.DstIP)
		}

		//fmt.Println()
	}
}

func sendDrumMessage(client *osc.Client, level int, speed int, instrument string) {
	msg := osc.NewMessage("/instrument/" + instrument)
	fmt.Println("sending", level, "to", instrument)
	msg.Append(int32(level))
	client.Send(msg)
}

func sendMelodyMessage(client *osc.Client, level int, speed int, instrument string) {
	msg := osc.NewMessage("/instrument/" + instrument)
	fmt.Println("sending", level, ",", speed, "to", instrument)
	msg.Append(int32(level))
	msg.Append(int32(speed))
	client.Send(msg)
}
