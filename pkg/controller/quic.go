package controller

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/fxamacker/cbor"
)

type QuicCommand uint8

const (
	QuicCmdInvalid QuicCommand = iota
	QuicCmdGet
	QuicCmdSet
	QuicCmdLog
	QuicCmdCalImu
	QuicCmdBlackbox
)

const (
	QuicFlagNone = iota
	QuicFlagError
)

const (
	QuicValInvalid = iota
	QuicValInfo
	QuicValProfile
	QuicValDefaultProfile
	QuicValBlackboxRate
	QuicValPidRatePresets
	QuicValVtxSettings
	QuicValOSDFont
)

type QuicPacket struct {
	cmd  QuicCommand
	flag uint8
	len  uint16

	Payload []byte
}

const quicHeaderLen = uint16(4)

var quicChannel = make(map[QuicCommand]chan QuicPacket)

var QuicLog = make(chan string, 100)
var QuicBlackbox = make(chan interface{}, 100)

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
	if cmd != QuicCmdBlackbox {
		log.Debugf("<quic> received cmd: %d flag: %d len: %d", cmd, flag, payloadLen)
	}

	payload, err := c.readAtLeast(int(payloadLen))
	if err != nil {
		return err
	}

	if len(payload) != int(payloadLen) {
		log.Debugf("% x (%s)", payload, string(payload))
		log.Fatalf("<quic> invalid size (%d vs %d)", length, size)
	}

	packet := QuicPacket{
		cmd:     cmd,
		flag:    flag,
		len:     payloadLen,
		Payload: payload,
	}

	switch packet.cmd {
	case QuicCmdLog:
		val := new(string)
		if err := cbor.Unmarshal(packet.Payload, val); err != nil {
			log.Fatal(err)
		}
		log.Debugf("<quic> log %s", *val)
		QuicLog <- *val
		break
	case QuicCmdBlackbox:
		val := new(Blackbox)
		if err := cbor.Unmarshal(packet.Payload, val); err != nil {
			log.Fatal(err)
		}
		QuicBlackbox <- *val
		break
	default:
		select {
		case quicChannel[cmd] <- packet:
		default:
		}
	}
	return nil
}

func (c *Controller) SendQUIC(cmd QuicCommand, data []byte) (*QuicPacket, error) {
	buf := []byte{
		'#',
		byte(cmd),
		byte((len(data) >> 8) & 0xFF),
		byte(len(data) & 0xFF),
	}
	buf = append(buf, data...)

	if quicChannel[cmd] == nil {
		quicChannel[cmd] = make(chan QuicPacket)
	}
	c.writeChannel <- buf

	log.Debugf("<quic> sent cmd: %d len: %d", cmd, len(data))

	select {
	case p := <-quicChannel[cmd]:
		if p.flag == QuicFlagError {
			var msg string
			if err := cbor.Unmarshal(p.Payload, &msg); err != nil {
				return nil, err
			}
			return nil, errors.New(msg)
		}
		return &p, nil
	case <-time.After(30 * time.Second):
		return nil, errors.New("quic recive timeout")
	}
}

func (c *Controller) GetQUICReader(typ QuicCommand) (io.Reader, error) {
	req, err := cbor.Marshal(typ, cbor.EncOptions{})
	if err != nil {
		return nil, err
	}

	p, err := c.SendQUIC(QuicCmdGet, req)
	if err != nil {
		return nil, err
	}

	dec := cbor.NewDecoder(bytes.NewReader(p.Payload))

	var inTyp QuicCommand
	if err := dec.Decode(&inTyp); err != nil {
		return nil, err
	}
	if typ != inTyp {
		return nil, fmt.Errorf("typ (%d) != inTyp (%d)", typ, inTyp)
	}

	return bytes.NewReader(p.Payload[dec.NumBytesRead():]), nil
}

func (c *Controller) GetQUIC(typ QuicCommand, v interface{}) error {
	r, err := c.GetQUICReader(typ)
	if err != nil {
		return err
	}

	if err := cbor.NewDecoder(r).Decode(v); err != nil {
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
	dec := cbor.NewDecoder(bytes.NewReader(p.Payload))
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

func (c *Controller) SetQUICReader(typ QuicCommand, r io.Reader, v interface{}) error {
	req := new(bytes.Buffer)

	enc := cbor.NewEncoder(req, cbor.EncOptions{})
	if err := enc.Encode(&typ); err != nil {
		return err
	}

	if _, err := io.Copy(req, r); err != nil {
		return err
	}

	p, err := c.SendQUIC(QuicCmdSet, req.Bytes())
	if err != nil {
		return err
	}

	var inTyp QuicCommand
	dec := cbor.NewDecoder(bytes.NewReader(p.Payload))
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
