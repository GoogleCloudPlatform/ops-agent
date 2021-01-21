package main

import (
	"log"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func main() {
	if err := main2(); err != nil {
		log.Fatal(err)
	}
}

func main2() error {
	name := "quentin1"
	desc := "Quentin Test 1"
	exepath := `C:\Program Files\td-agent-bit\bin\fluent-bit.exe` // `C:\dev\install1\exec.bat`

	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err == nil {
		defer s.Close()
		s.Control(svc.Stop)
		// TODO: check that stop worked/wait for stop
		if err := s.Delete(); err != nil {
			return err
		}
		// FIXME: Delete doesn't succeed until Service Manager is closed
		//return fmt.Errorf("service %s already exists", name)
	}
	s, err = m.CreateService(name, exepath, mgr.Config{DisplayName: desc}, "-c", `C:\dev\install1\fluentbit.conf`)
	if err != nil {
		return err
	}
	defer s.Close()
	return nil
}
