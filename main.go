package main

import (
	"encoding/json"
	"errors"
	"log"
)

const (
	timeMultiplier = 1000
	hopPenalty     = 15 / 255
)

type Network struct {
	Nodes map[string]*Node
	Edges map[string]*Edge
}

type Node struct {
	*Network
	Address       string
	Originators   map[string]*Originator
	Neighbors     map[string]*Neighbor
	OgmSequence   int
	PacketChannel chan (Packet)
}

type Neighbor struct {
	Address    string
	Throughput int
}

type Originator struct {
	Address string
	NextHop struct {
		Address    string
		Throughput int
	}
	OgmSequence int
}

type Edge struct {
	Throughput int
	Bucket     int
}

type OGM struct {
	Sequence          int
	OriginatorAddress string
	SenderAddress     string
	Throughput        int
	Timestamp         int
}

type Packet struct {
	Type    string
	Payload []byte
}

// func (router *Node) BroadcastOgm() {
// 	router.OgmSequence++
// 	for address, _ := range router {
// 		router.
// 	}
// }

func (edge *Edge) SendBits(num int) bool {
	newBucket := edge.Bucket + num
	if newBucket > edge.Throughput {
		edge.Bucket = edge.Throughput
		return false
	}
	edge.Bucket = newBucket
	return true
}

func (edge *Edge) Tick(ticksPerSecond int) {
	if edge.Bucket > 0 {
		edge.Bucket -= edge.Throughput / ticksPerSecond
	}
}

func (node *Node) SendPacket(destAddress string, packetType string, payload []byte) error {
	dest := node.Network.Nodes[destAddress]
	if dest == nil {
		return errors.New("destination not found")
	}

	edge := node.Network.Edges[node.Address+"->"+dest.Address]
	if dest == nil {
		return errors.New("edge not found")
	}

	if edge.SendBits(len(payload)) {
		dest.ReceivePacket(Packet{packetType, payload})
	}

	return nil
}

func (node *Node) Listen() {
	for {
		packet := <-node.PacketChannel
		var err error
		switch packet.Type {
		case "OGM":
			err = node.HandleOGM(packet.Payload)
		}

		if err != nil {
			log.Println(node.Address, err)
		}
	}
}

func (node *Node) HandleOGM(payload []byte) error {
	ogm := OGM{}
	err := json.Unmarshal(payload, ogm)
	if err != nil {
		return err
	}

	if ogm.OriginatorAddress == node.Address {
		return nil
	}

	adjustedOGM, err := node.AdjustOGM(ogm)
	if err != nil {
		return err
	}

	err = node.UpdateOriginator(*adjustedOGM)
	if err != nil {
		return err
	}

	node.RebroadcastOGM(*adjustedOGM)
	return nil
}

func (node *Node) AdjustOGM(ogm OGM) (*OGM, error) {
	/* Update the received throughput metric to match the link
	 * characteristic:
	 *  - If this OGM traveled one hop so far (emitted by single hop
	 *    neighbor) the path throughput metric equals the link throughput.
	 *  - For OGMs traversing more than one hop the path throughput metric is
	 *    the smaller of the path throughput and the link throughput.
	 */
	neighbor := node.Neighbors[ogm.SenderAddress]
	if neighbor == nil {
		return nil, errors.New("OGM not sent from neighbor")
	}

	if ogm.OriginatorAddress == ogm.SenderAddress {
		ogm.Throughput = neighbor.Throughput
	} else {
		if neighbor.Throughput < ogm.Throughput {
			ogm.Throughput = neighbor.Throughput
		}
	}

	ogm.Throughput = ogm.Throughput - (ogm.Throughput * hopPenalty)
	return &ogm, nil
}

func (node *Node) UpdateOriginator(ogm OGM) error {
	originator := node.Originators[ogm.OriginatorAddress]

	if originator == nil {
		originator := Originator{
			OgmSequence: ogm.Sequence,
			Address:     ogm.OriginatorAddress,
		}
		originator.NextHop.Address = ogm.SenderAddress
		originator.NextHop.Throughput = ogm.Throughput

		node.Originators[ogm.OriginatorAddress] = &originator
	} else if ogm.Sequence > originator.OgmSequence {
		if originator.NextHop.Throughput < ogm.Throughput {
			originator.NextHop.Address = ogm.SenderAddress
			originator.NextHop.Throughput = ogm.Throughput
		}
	} else {
		return errors.New("ogm sequence too low")
	}
	return nil
}

func (node *Node) RebroadcastOGM(ogm OGM) error {
	ogm.SenderAddress = node.Address
	payload, err := json.Marshal(ogm)
	if err != nil {
		return err
	}
	for _, neighbor := range node.Neighbors {
		err = node.SendPacket(neighbor.Address, "OGM", payload)
		if err != nil {
			return err
		}
	}
	return nil
}
