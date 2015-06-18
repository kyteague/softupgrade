package softupgrade

import (
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func TestSoftUpgrade(t *testing.T) {
	go func() {
		f, err := ioutil.TempFile("/tmp", "softupgrade")
		if err != nil {
			t.Fatal(err.Error())
		}
		defer f.Close()
		wconn, err := Listen("test")
		if err != nil {
			t.Fatal(err.Error())
		}

		err = wconn.Send([]byte("hello"), []*os.File{f})
		if err != nil {
			t.Fatal(err.Error())
		}
	}()

	// Allow server to boot
	time.Sleep(200 * time.Millisecond)

	rconn, err := Dial(os.Getpid(), "test")
	if err != nil {
		t.Fatal(err.Error())
	}

	data, files, err := rconn.Recv()
	if err != nil {
		t.Fatal(err.Error())
	}

	if string(data) != "hello" {
		t.Errorf("Expected hello got %v", string(data))
	}

	if len(files) != 1 {
		t.Errorf("Expected 1 file got %v", len(files))
	}
}
