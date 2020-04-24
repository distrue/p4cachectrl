package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"reflect"
	"strconv"
	"strings"
	"time"

	cuckoo "github.com/distrue/gencachectrl/cuckoo"
	p4Config "github.com/distrue/gencachectrl/p4/config/v1"
	p4 "github.com/distrue/gencachectrl/p4/v1"
	gopacket "github.com/google/gopacket"
	layers "github.com/google/gopacket/layers"
)

func l4_add(client p4.P4RuntimeClient, tableId uint32, matchId uint32, egress uint32, ipv4 []byte, mac []byte, port byte) {
	var tableEntry p4.TableEntry
	tableEntry.TableId = tableId
	matchList := make([]*p4.FieldMatch, 1)
	LPMMatch := p4.FieldMatch_LPM{Value: ipv4, PrefixLen: 32}
	matchList[0] = &p4.FieldMatch{
		FieldId:        matchId,
		FieldMatchType: &p4.FieldMatch_Lpm{Lpm: &LPMMatch},
	}
	egressParam := p4.Action_Param{ParamId: 1, Value: mac}
	egressParam2 := p4.Action_Param{ParamId: 2, Value: []byte{0x00, port}}

	// 2 action params
	paramList := make([]*p4.Action_Param, 2)
	paramList[0] = &egressParam
	paramList[1] = &egressParam2
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
		if table.Preamble.GetName() == "MyIngress.ipv4_lpm" {
			tableId = table.Preamble.GetId()
			for _, match := range table.MatchFields {
				if match.GetName() == "hdr.ipv4.dstAddr" {
					matchId = match.GetId()
				}
			}
			// MyIngress.l2_forward find
			for _, action := range cfg.Actions {
				if action.Preamble.GetName() == "MyIngress.ipv4_forward" {
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

func setTable(client p4.P4RuntimeClient, mac string) {
	cfg, err := p4.GetPipelineConfigs(client)
	if err != nil {
		log.Fatal(err)
	}

	tableId, matchId, egress := parseL2ForwardFromCfg(cfg)
	fmt.Printf("%v %v %v\n", tableId, matchId, egress)

	idx := byte(1)
	for idx <= byte(9) {
		ipmatch := []byte{0x0a, 0x00, 0x00, idx}
		mac := []byte{0x00, 0x00, 0x0a, 0x00, 0x00, idx}
		port := idx
		l4_add(client, tableId, matchId, egress, ipmatch, mac, port)
		idx = idx + byte(1)
	}

	x := strings.Split(mac, ":")
	y := make([]byte, 6)
	for idx, it := range x {
		num, err := strconv.Atoi(it)
		n := byte(num)
		y[idx] = n
	}
	l4_add(client, tableId, matchId, egress, []byte{0x0a, 0x00, 0x00, 0x0a}, y, 0xc8)
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

type Node map[string]interface{}

func NewNode(target interface{}) Node {
	data := target.(map[string]interface{})
	return data
}

func JSONFinder(bundle interface{}, path []string) interface{} {
	if len(path) == 0 {
		return bundle
	}
	data := NewNode(bundle)
	rt := reflect.TypeOf(data)
	if rt.Kind() != reflect.Map {
		return nil
	}
	for key, item := range data {
		if key == path[0] {
			return JSONFinder(item, path[1:])
		}
	}
	return nil
}

func main() {
	/*
		1. connect to P4Runtime
		2. set Role of connection
		3. Packet I/O for gencache
		4. listen
	*/

	file, err := ioutil.ReadFile("./topology.db")
	if err != nil {
		fmt.Println(err)
		log.Fatalf("cannot read topology file")
	}

	var data interface{}
	err = json.Unmarshal(file, &data)
	if err != nil {
		fmt.Println(err)
	}
	mac := JSONFinder(data, []string{"s1", "sw-cpu", "mac"})

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
	// sudo p4c --target bmv2 --arch v1model --std p4-16 "gencache.p4" --p4runtime-files p4info.txt
	p4.SetPipelineConfigFromFile(client, "p4info.txt")
	// p4.PrintTables(client)
	setTable(client, mac.(string)) // Entry Setup
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
