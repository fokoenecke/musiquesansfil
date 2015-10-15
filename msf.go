package main

import (
	"flag"
	"fmt"
	"log"
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
	packets int
	since   time.Time
}

func (self *Activity) increment() {
	self.packets++
}

func (self *Activity) currentPackets() int {
	return self.packets
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
	var m = make(map[string]*Activity)

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
		}

		ip4Layer := packet.Layer(layers.LayerTypeIPv4)
		if ip4Layer != nil {
			var ip = ip4Layer.(*layers.IPv4)
			_, ok := m[ip.SrcIP.String()]
			if ok {
				p := m[ip.SrcIP.String()]
				p.increment()
			} else {
				m[ip.SrcIP.String()] = &Activity{1, time.Now()}
			}
			_, ok = m[ip.DstIP.String()]
			if ok {
				p := m[ip.DstIP.String()]
				p.increment()
			} else {
				m[ip.DstIP.String()] = &Activity{1, time.Now()}
			}
			fmt.Println("Source IP: ", ip.SrcIP)
			fmt.Println("Destination IP: ", ip.DstIP)
		}

		c := 0
		for key, value := range m {
			elapsed := time.Since(value.since)
			pps := float64(value.currentPackets()) / elapsed.Seconds()
			if c == 0 {
				if pps > 2 {
					sendMessage(client, 4)
				} else if pps > 1.5 {
					sendMessage(client, 3)
				} else if pps > 1.0 {
					sendMessage(client, 2)
				} else if pps > 0.5 {
					sendMessage(client, 1)
				} else {
					sendMessage(client, 0)
					delete(m, key)
				}
			}
			c++
			fmt.Println("Key:", key, "elapsed:", elapsed, "Packets:", value.currentPackets(), "pps:", pps)
		}
		fmt.Println()
	}
}

func sendMessage(client *osc.Client, level int) {
	msg := osc.NewMessage("/instrument/kick")
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
