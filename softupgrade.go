package softupgrade

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"syscall"
)

var PathPrefix = "/tmp"

type UpgradeConn struct {
	c *net.UnixConn
}

func Listen(name string) (*UpgradeConn, error) {
	laddr, err := makeAddr(os.Getpid(), name)
	if err != nil {
		return nil, err
	}

	l, err := net.ListenUnix("unix", laddr)
	if err != nil {
		return nil, err
	}

	c, err := l.AcceptUnix()
	if err != nil {
		return nil, err
	}

	return &UpgradeConn{c: c}, nil
}

func Dial(pid int, name string) (*UpgradeConn, error) {
	raddr, err := makeAddr(pid, name)
	if err != nil {
		return nil, err
	}

	c, err := net.DialUnix("unix", nil, raddr)
	if err != nil {
		return nil, err
	}

	return &UpgradeConn{c: c}, nil
}

func (c *UpgradeConn) Send(data []byte, files []*os.File) error {
	fds := make([]int, len(files))
	for i, f := range files {
		fds[i] = int(f.Fd())
	}
	rights := syscall.UnixRights(fds...)

	var dataSize uint32 = uint32(len(data))
	var oobSize uint32 = uint32(len(rights))

	// First send down sizes
	err := binary.Write(c.c, binary.BigEndian, dataSize)
	if err != nil {
		return err
	}
	err = binary.Write(c.c, binary.BigEndian, oobSize)
	if err != nil {
		return err
	}

	_, _, err = c.c.WriteMsgUnix(data, rights, nil)
	return err
}

func (c *UpgradeConn) Recv() ([]byte, []*os.File, error) {
	var dataSize, oobSize uint32

	err := binary.Read(c.c, binary.BigEndian, &dataSize)
	if err != nil {
		return nil, nil, err
	}
	err = binary.Read(c.c, binary.BigEndian, &oobSize)
	if err != nil {
		return nil, nil, err
	}

	data := make([]byte, dataSize)
	oob := make([]byte, oobSize)

	_, oobn, _, _, err := c.c.ReadMsgUnix(data, oob)
	if err != nil {
		return nil, nil, err
	}

	scms, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return nil, nil, err
	}
	if len(scms) != 1 {
		return nil, nil, fmt.Errorf("Recv: expected 1 SCM, got %d", len(scms))
	}

	fds, err := syscall.ParseUnixRights(&scms[0])
	if err != nil {
		return nil, nil, err
	}

	files := make([]*os.File, len(fds))
	for i, fd := range fds {
		files[i] = os.NewFile(uintptr(fd), fmt.Sprintf("fd %d", fd))
	}

	return data, files, nil
}

func (c *UpgradeConn) Close() error {
	return c.c.Close()
}

func makeAddr(pid int, name string) (*net.UnixAddr, error) {
	addr := fmt.Sprintf("%s/softupgrade_%d_%s", PathPrefix, pid, name)
	return net.ResolveUnixAddr("unix", addr)
}
