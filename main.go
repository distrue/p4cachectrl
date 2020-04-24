package main

import (
	"context"
	"fmt"
	"log"
	"time"

	cuckoo "github.com/distrue/gencachectrl/cuckoo"
	p4Config "github.com/distrue/gencachectrl/p4/config/v1"
	p4 "github.com/distrue/gencachectrl/p4/v1"
	gopacket "github.com/google/gopacket"
	layers "github.com/google/gopacket/layers"
)

func l2_add(client p4.P4RuntimeClient, tableId uint32, matchId uint32, egress uint32, val byte, port byte) {
	var tableEntry p4.TableEntry
	tableEntry.TableId = tableId
	matchList := make([]*p4.FieldMatch, 1)
	ExactMatch := p4.FieldMatch_Exact{Value: []byte{0x00, 0x00, 0x0a, 0x00, 0x00, val}}
	matchList[0] = &p4.FieldMatch{
		FieldId:        matchId,
		FieldMatchType: &p4.FieldMatch_Exact_{Exact: &ExactMatch},
	}
	egressParam := p4.Action_Param{ParamId: 1, Value: []byte{0x00, port}}

	// 2 action params
	paramList := make([]*p4.Action_Param, 1)
	paramList[0] = &egressParam
	action := p4.Action{ActionId: egress, Params: paramList}
	tableAction := p4.TableAction_Action{Action: &action}
	tableEntry.Match = matchList
	tableEntry.Action = &p4.TableAction{Type: &tableAction}

	// Ship this table entry to the RT agent in the switch
	tabEntity := &p4.Entity_TableEntry{TableEntry: &tableEntry}
	entity := &p4.Entity{Entity: tabEntity}
	updates := make([]*p4.Update, 1)
	singleUpdate := &p4.Update{Type: p4.Update_INSERT, Entity: entity}
	fmt.Printf("%v\n", singleUpdate)
	updates[0] = singleUpdate
	_, errw := client.Write(context.Background(),
		&p4.WriteRequest{DeviceId: 1,
			ElectionId: &p4.Uint128{High: 10000, Low: 9999},
			Updates:    updates})
	if errw != nil {
		fmt.Println(errw)
		log.Fatalf("Error sending table entry to rt agent in switch")
	}
	fmt.Printf("success -\n")
}

func parseL2ForwardFromCfg(cfg *p4Config.P4Info) (uint32, uint32, uint32) {
	var tableId uint32 = 0
	var matchId uint32 = 0
	var egress uint32 = 0

	for _, table := range cfg.Tables {
		if table.Preamble.GetName() == "MyIngress.l2_forward" {
			tableId = table.Preamble.GetId()
			for _, match := range table.MatchFields {
				if match.GetName() == "hdr.ethernet.dstAddr" {
					matchId = match.GetId()
				}
			}
			// MyIngress.l2_forward find
			for _, action := range cfg.Actions {
				if action.Preamble.GetName() == "MyIngress.set_egress_port" {
					egress = action.Preamble.GetId()
				}
			}
		}
	}

	return tableId, matchId, egress
}

func parseLookupFromCfg(cfg *p4Config.P4Info) (uint32, uint32, uint32) {
	var tableId uint32 = 0
	var matchId uint32 = 0
	var egress uint32 = 0

	for _, table := range cfg.Tables {
		if table.Preamble.GetName() == "MyIngress.lookup_table" {
			tableId = table.Preamble.GetId()
			for _, match := range table.MatchFields {
				if match.GetName() == "hdr.gencache.key" {
					matchId = match.GetId()
				}
			}
			// MyIngress.l2_forward find
			for _, action := range cfg.Actions {
				if action.Preamble.GetName() == "MyIngress.set_lookup_metadata" {
					egress = action.Preamble.GetId()
				}
			}
		}
	}

	return tableId, matchId, egress
}

func setTable(client p4.P4RuntimeClient) {
	cfg, err := p4.GetPipelineConfigs(client)
	if err != nil {
		log.Fatal(err)
	}

	tableId, matchId, egress := parseL2ForwardFromCfg(cfg)
	fmt.Printf("%v %v %v\n", tableId, matchId, egress)

	idx := byte(1)
	for idx <= byte(9) {
		l2_add(client, tableId, matchId, egress, idx, idx)
		idx = idx + byte(1)
	}
	l2_add(client, tableId, matchId, egress, 0x0a, 0xc8)
}

func gencache_func(src []byte, filter *cuckoo.Cuckoo, stream p4.P4Runtime_StreamChannelClient) {
	// ongoing
	parse := gopacket.NewPacket(src, layers.LayerTypeIPv4, gopacket.Default)
	fmt.Printf("%v \n", parse.Layers())
	/*
		parsed := gopacket.NewPacket(packet.Payload, gopacket.layers.IPv4, gopacket.Default)
		if gopacket.layers.TCP.canDecode(parsed.payload) {
			// tcp decode
		}
		if gopacket.layers.UDP.canDecode(parsed.payload) {
			// udp decode
		}
	*/
	// Resend to 10.0.0.1
	src[5] = 1

	// PacketOut
	req := p4.StreamMessageRequest{
		Update: &p4.StreamMessageRequest_Packet{
			Packet: &p4.PacketOut{
				Payload: src,
			},
		},
	}
	err := stream.Send(&req)
	if err != nil {
		log.Fatal(err)
	}
}

func print_cfg(client p4.P4RuntimeClient) {
	newConfig, err := p4.GetPipelineConfigs(client)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", newConfig)
}

func main() {
	/*
		1. connect to P4Runtime
		2. set Role of connection
		3. Packet I/O for gencache
		4. listen
	*/

	// 1. connect to P4Runtime
	client := p4.GetClient("localhost:50051")
	stream, sErr := client.StreamChannel(context.Background())
	if sErr != nil {
		fmt.Println(sErr)
		log.Fatalf("cannot open stream channel with the server")
	}
	listener := p4.OpenStreamListener(stream)

	// 2. set Role of connection
	p4.SetMastership(stream)
	p4.SetPipelineConfigFromFile(client, "resources/p4info.txt")
	// p4.PrintTables(client)
	setTable(client) // Entry Setup
	time.Sleep(1000 * time.Millisecond)

	// 3. Packet I/O for gencache
	filter := cuckoo.NewCuckoo(10, 0.1)
	for {
		// PacketIn
		res, err := stream.Recv()
		if err != nil {
			log.Fatal(err)
		}
		packet := res.GetPacket()
		fmt.Printf("%+v\n", packet.Payload)

		gencache_func(packet.Payload, filter, stream)
	}

	// 4. listen
	listener.Wait()

}
