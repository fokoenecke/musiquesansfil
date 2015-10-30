package main

import (
	"flag"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/hypebeast/go-osc/osc"
)

var serverHost = flag.String("s", "127.0.0.1", "Set Pure Data server host")
var serverPort = flag.Int("p", 9001, "Set Pure Data server port")
var device = flag.String("d", "wlan0", "Set device to listen on")

type Activity struct {
	packets    int
	sizeSum    int
	since      time.Time
	instrument int
}

func (activity *Activity) increment() {
	activity.packets++
}

func (activity *Activity) currentPackets() int {
	return activity.packets
}

func (activity *Activity) addPacketSize(size int) {
	activity.sizeSum += size
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
		m              map[string]*Activity
		instrumentPool []int
	}{m: make(map[string]*Activity)}

	instruments := map[int]string{
		0: "kick",
		1: "snare",
		2: "hh",
		3: "bass",
	}

	go func() {
		for {

			clients.Lock()

			for key, value := range clients.m {
				elapsed := time.Since(value.since)
				pps := float64(value.currentPackets()) / elapsed.Seconds()
				bps := float64(value.sizeSum) / elapsed.Seconds()
				instrument, ok := instruments[value.instrument]
				if ok {
					if bps > 200 {
						sendMessage(client, 4, instrument)
					} else if bps > 150 {
						sendMessage(client, 3, instrument)
					} else if bps > 50 {
						sendMessage(client, 2, instrument)
					} else if bps > 20 {
						sendMessage(client, 1, instrument)
					} else {
						sendMessage(client, 0, instrument)
						clients.instrumentPool = append(clients.instrumentPool, value.instrument)
						sort.Ints(clients.instrumentPool)
						delete(clients.m, key)
					}
				}
				fmt.Println("Key:", key, "instrument:", value.instrument, "elapsed:", elapsed, "Packets:", value.currentPackets(), "pps:", pps, "bps:", bps)
			}
			if len(clients.m) > 0 {
				fmt.Println()
			}

			clients.Unlock()
			time.Sleep(2 * time.Second)
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
				clients.m[ethernetPacket.SrcMAC.String()] = &Activity{1, packetLength, time.Now(), instrument}
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
				clients.m[ethernetPacket.DstMAC.String()] = &Activity{1, packetLength, time.Now(), instrument}
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

func sendMessage(client *osc.Client, level int, instrument string) {
	msg := osc.NewMessage("/instrument/" + instrument)
	fmt.Println("sending", level, "to", instrument)
	msg.Append(int32(level))
	client.Send(msg)
}

func playSound(client *osc.Client) {
	msg := osc.NewMessage("/instrument/kick")
	msg.Append(int32(1))
	client.Send(msg)
	time.Sleep(50 * time.Millisecond)
	msg = osc.NewMessage("/instrument/kick")
	msg.Append(int32(1))
	client.Send(msg)
}
