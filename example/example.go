package main

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"io"
	"log"
	"net"
	"net/textproto"
	"os"
	"softupgrade"
	"strconv"
)

const pidPath = "example.pid"

type session struct {
	name string
	c    net.Conn
}

var sessions []*session
var ln net.Listener

func readPid() (int, error) {
	f, err := os.OpenFile(pidPath, os.O_RDONLY, 0600)
	if err != nil {
		return -1, err
	}
	defer f.Close()

	var buf bytes.Buffer
	_, err = buf.ReadFrom(f)
	if err != nil {
		return -1, err
	}

	parentPid, _ := strconv.Atoi(buf.String())
	return parentPid, nil
}

func writePid() error {
	f, err := os.OpenFile(pidPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	defer f.Close()

	f.WriteString(strconv.Itoa(os.Getpid()))
	f.Close()
	return nil
}

func main() {
	log.Print("Starting server")

	var err error
	parentPid, _ := readPid()
	writePid()

	if parentPid > 0 {
		err = upgrade(parentPid)
		if err != nil {
			log.Printf("upgrade: %s", err)
		}
	}

	if ln == nil {
		ln, err = net.Listen("tcp", ":9654")
		if err != nil {
			log.Fatalf("Error listening: %s", err)
		}
	}

	go func() {
		err := downgrade()
		if err != nil {
			log.Printf("downgrade: %s", err)
			return
		}
	}()
	serve(ln)
}

func downgrade() error {
	c, err := softupgrade.Listen("example")
	if err != nil {
		return err
	}
	defer c.Close()

	log.Print("Starting downgrade")
	lnf, err := ln.(*net.TCPListener).File()
	if err != nil {
		return err
	}
	defer lnf.Close()
	ln.Close()

	err = c.Send(nil, []*os.File{lnf})
	if err != nil {
		return err
	}

	for _, s := range sessions {
		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)
		enc.Encode(s)
		f, _ := s.c.(*net.TCPConn).File()
		defer f.Close()
		s.c.Close()
		c.Send(buf.Bytes(), []*os.File{f})
	}
	log.Print("Finished downgrade")
	return nil
}

func upgrade(pid int) error {
	c, err := softupgrade.Dial(pid, "example")
	if err != nil {
		return err
	}

	log.Print("Starting upgrade")

	_, files, err := c.Recv()
	if err != nil {
		return err
	}

	lnf := files[0]
	defer lnf.Close()

	ln, err = net.FileListener(lnf)
	if err != nil {
		return err
	}

	for {
		data, files, err := c.Recv()
		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		f := files[0]
		defer f.Close()

		var s session
		decoder := gob.NewDecoder(bytes.NewBuffer(data))
		err = decoder.Decode(&s)
		if err != nil {
			return err
		}
		s.c, _ = net.FileConn(f)
		sessions = append(sessions, &s)

		go work(&s)
	}

	log.Print("Finished upgrade")
	return nil
}

func serve(l net.Listener) {
	for {
		c, err := l.Accept()
		if err != nil {
			log.Printf("Accept error: %s", err)
			return
		}

		s := &session{c: c}
		sessions = append(sessions, s)

		go work(s)
	}
}

func work(s *session) {
	w := textproto.NewWriter(bufio.NewWriter(s.c))
	r := textproto.NewReader(bufio.NewReader(s.c))

	if s.name == "" {
		name, err := r.ReadLine()
		if err != nil {
			log.Printf("error reading name: %s", err)
			return
		}

		log.Printf("%s connected", name)
		s.name = name
		w.PrintfLine("Hello %s!", name)
	}

	for {
		data, err := r.ReadLine()
		if err != nil {
			log.Printf("connection error: %s", err)
			return
		}
		log.Printf("RECV: %s", data)
		w.PrintfLine("PONG!")
	}
}

func (s *session) GobEncode() ([]byte, error) {
	var buf bytes.Buffer
	e := gob.NewEncoder(&buf)
	e.Encode(s.name)

	return buf.Bytes(), nil
}

func (s *session) GobDecode(b []byte) error {
	d := gob.NewDecoder(bytes.NewBuffer(b))
	return d.Decode(&s.name)
}
