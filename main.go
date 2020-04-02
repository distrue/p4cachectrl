package main

import (
	"context"
	"fmt"
	"log"
	"time"

	p4 "github.com/distrue/gencachectrl/p4/v1"
)

func l2_add(client p4.P4RuntimeClient, tableId uint32, matchId uint32, egress uint32, val byte, port byte) {
	ans, err := client.Write(context.Background(), &p4.WriteRequest{
		DeviceId: 1,
		Updates: []*p4.Update{
			&p4.Update{
				Type: p4.Update_INSERT,
				Entity: &p4.Entity{
					Entity: &p4.Entity_TableEntry{
						TableEntry: &p4.TableEntry{
							TableId: tableId,
							Match: []*p4.FieldMatch{
								&p4.FieldMatch{
									FieldId: matchId,
									FieldMatchType: &p4.FieldMatch_Exact_{
										Exact: &p4.FieldMatch_Exact{
											Value: []byte{0x00, 0x00, 0x0a, 0x00, 0x00, val},
										},
									},
								},
							},
							Action: &p4.TableAction{
								Type: &p4.TableAction_Action{
									Action: &p4.Action{
										ActionId: egress,
										Params: []*p4.Action_Param{
											&p4.Action_Param{
												ParamId: egress,
												Value:   []byte{port, 0x00},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		fmt.Printf("error occured (env: %v)", val)
		log.Fatal(err)
		return
	}
	fmt.Printf("success - %v\n", ans.String())
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
				if action.Preamble.GetName() == "set_egress_port" {
					egress = action.Preamble.GetId()
				}
			}
		}
	}
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
