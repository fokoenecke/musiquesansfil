package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/hypebeast/go-osc/osc"
)

var serverHost = flag.String("s", "127.0.0.1", "Set Pure Data server host")
var serverPort = flag.Int("p", 9001, "Set Pure Data server port")

func main() {
	flag.Parse()
	if handle, err := pcap.OpenLive("wlan0", 65536, true, 0); err != nil {
		defer handle.Close()
		panic(err)
	} else {
		fmt.Println(handle.LinkType())
		packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
		client := osc.NewClient(*serverHost, *serverPort)

		for packet := range packetSource.Packets() {
			go playSound(client)
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
				fmt.Println("Source IP: ", ip.SrcIP)
				fmt.Println("Destination IP: ", ip.DstIP)
			}
			
			fmt.Println()
		}
	}
}

func playSound(client *osc.Client) {
	msg := osc.NewMessage("/midi/noteon")
	msg.Append(int32(64))
	client.Send(msg)
	time.Sleep(50 * time.Millisecond)
	msg = osc.NewMessage("/midi/noteoff")
	msg.Append(int32(64))
	client.Send(msg)
}
