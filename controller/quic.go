package controller

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/fxamacker/cbor"
)

type QuicCommand uint8

const (
	QuicCmdInvalid QuicCommand = iota
	QuicCmdGet
	QuicCmdSet
	QuicCmdLog
)

const (
	QuicFlagNone = iota
	QuicFlagError
)

const (
	QuicValInvalid = iota
	QuicValProfile
	QuicValRx
	QuicValVbat
	QuicValInfo
	QuicValDefaultProfile
	QuicValGyro
)

type quicPacket struct {
	cmd     QuicCommand
	flag    uint8
	len     uint16
	payload []byte
}

const quicHeaderLen = uint16(4)

var quicChannel = make(map[QuicCommand]chan quicPacket)

func (c *Controller) ReadQUIC() error {
	header, err := c.readAtLeast(int(quicHeaderLen - 1))
	if err != nil {
		return err
	}
	length := int(quicHeaderLen)

	cmd := QuicCommand(header[0] & (0xff >> 3))
	flag := (header[0] >> 5)
	payloadLen := uint16(header[1])<<8 | uint16(header[2])
	size := int(quicHeaderLen + payloadLen)

	payload, err := c.readAtLeast(int(payloadLen))
	if err != nil {
		return err
	}

	if len(payload) != int(payloadLen) {
		log.Printf("% x (%s)\n", payload, string(payload))
		log.Fatalf("<quic> invalid size (%d vs %d)", length, size)
	}

	packet := quicPacket{
		cmd:     cmd,
		flag:    flag,
		len:     payloadLen,
		payload: payload,
	}
	log.Printf("<quic> received cmd: %d flag: %d len: %d\n", packet.cmd, packet.flag, packet.len)

	if packet.cmd == QuicCmdLog {
		val := new(string)
		if err := cbor.Unmarshal(packet.payload, val); err != nil {
			log.Fatal(err)
		}
		log.Printf("<quic> log %s\n", *val)
	} else {
		quicChannel[cmd] <- packet
	}
	return nil
}

func (c *Controller) SendQUIC(cmd QuicCommand, data []byte) (*quicPacket, error) {
	buf := []byte{
		'#',
		byte(cmd),
		byte((len(data) >> 8) & 0xFF),
		byte(len(data) & 0xFF),
	}
	buf = append(buf, data...)

	if quicChannel[cmd] == nil {
		quicChannel[cmd] = make(chan quicPacket)
	}
	c.writeChannel <- buf

	select {
	case p := <-quicChannel[cmd]:
		return &p, nil
	case <-time.After(10 * time.Second):
		return nil, errors.New("quic recive timeout")
	}
}

func (c *Controller) GetQUIC(typ QuicCommand, v interface{}) error {
	req, err := cbor.Marshal(typ, cbor.EncOptions{})
	if err != nil {
		return err
	}

	p, err := c.SendQUIC(QuicCmdGet, req)
	if err != nil {
		return err
	}

	dec := cbor.NewDecoder(bytes.NewReader(p.payload))
	if p.flag == QuicFlagError {
		var msg string
		if err := dec.Decode(&msg); err != nil {
			return err
		}
		return errors.New(msg)
	}

	var inTyp QuicCommand
	if err := dec.Decode(&inTyp); err != nil {
		return err
	}
	if typ != inTyp {
		return fmt.Errorf("typ (%d) != inTyp (%d)", typ, inTyp)
	}

	if err := dec.Decode(v); err != nil {
		return err
	}

	return nil
}

func (c *Controller) SetQUIC(typ QuicCommand, v interface{}) error {
	req := new(bytes.Buffer)

	enc := cbor.NewEncoder(req, cbor.EncOptions{})
	if err := enc.Encode(&typ); err != nil {
		return err
	}
	if err := enc.Encode(v); err != nil {
		return err
	}

	p, err := c.SendQUIC(QuicCmdSet, req.Bytes())
	if err != nil {
		return err
	}

	var inTyp QuicCommand
	dec := cbor.NewDecoder(bytes.NewReader(p.payload))
	if err := dec.Decode(&inTyp); err != nil {
		return err
	}
	if typ != inTyp {
		return fmt.Errorf("typ (%d) != inTyp (%d)", typ, inTyp)
	}

	if err := dec.Decode(v); err != nil {
		return err
	}

	return nil
}
