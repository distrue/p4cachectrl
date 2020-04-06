package main

import (
	"context"
	"fmt"
	"log"
	"time"

	p4 "github.com/distrue/gencachectrl/p4/v1"
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

	//fmt.Printf("%+v", tableEntry)

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

func setTable(client p4.P4RuntimeClient) {
	cfg, err := p4.GetPipelineConfigs(client)
	if err != nil {
		log.Fatal(err)
	}

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
	fmt.Printf("%v %v %v\n", tableId, matchId, egress)
	l2_add(client, tableId, matchId, egress, 0x01, 0x01)
	l2_add(client, tableId, matchId, egress, 0x02, 0x02)
	l2_add(client, tableId, matchId, egress, 0x03, 0x03)
	l2_add(client, tableId, matchId, egress, 0x04, 0x04)
	l2_add(client, tableId, matchId, egress, 0x05, 0x05)
	l2_add(client, tableId, matchId, egress, 0x06, 0x06)
	l2_add(client, tableId, matchId, egress, 0x07, 0x07)
	l2_add(client, tableId, matchId, egress, 0x08, 0x08)
	l2_add(client, tableId, matchId, egress, 0x09, 0x09)
	l2_add(client, tableId, matchId, egress, 0x0a, 0xc8)
}

func main() {

	client := p4.GetClient("localhost:50051")
	stream, sErr := client.StreamChannel(context.Background())
	if sErr != nil {
		fmt.Println(sErr)
		log.Fatalf("cannot open stream channel with the server")
	}
	listener := p4.OpenStreamListener(stream)

	p4.SetMastership(stream)

	p4.SetPipelineConfigFromFile(client, "resources/p4info.txt")

	p4.PrintTables(client)

	// Entry Setup
	setTable(client)

	time.Sleep(1000 * time.Millisecond)

	// PacketIn Receiver
	var sum = 0
	for sum < 1000 {
		res, err := stream.Recv()
		if err != nil {
			log.Fatal(err)
		}
		packet := res.GetPacket()
		fmt.Printf("%+v\n", packet.Payload)
		sum += 1
	}

	// Config Print
	/*
		newConfig, err := p4.GetPipelineConfigs(client)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%+v\n", newConfig)
	*/

	// Packet Out - controller instance to switch
	/*
		req := p4.StreamMessageRequest{
			Update: &p4.StreamMessageRequest_Packet{
				Packet: &p4.PacketOut{
					Payload: []byte("PAYLOAD"), // []byte
					Metadata: []*p4.PacketMetadata{
						&p4.PacketMetadata{},
					},
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     []byte("unrecognized"), // []byte
					XXX_sizecache:        2048,
				},
			},
		}

		err = stream.Send(&req)
		if err != nil {
			fmt.Println("ERROR SENDING STREAM REQUEST:")
			fmt.Println(err)
		}
	*/

	// PacketIn - switch to controller instance
	// ans, err := stream.Recv()
	// fmt.Printf("%+v\n", ans)

	listener.Wait()

}
