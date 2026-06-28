package shellcommand

import "testing"

func TestCheckHardlineCommandBlocksDisasterCommands(t *testing.T) {
	cases := []struct {
		name    string
		command string
	}{
		{name: "recursive root delete", command: "rm -rf /"},
		{name: "recursive system delete", command: "rm -rf /etc"},
		{name: "recursive home delete", command: "rm -rf ~"},
		{name: "format filesystem", command: "mkfs.ext4 /dev/sda1"},
		{name: "raw disk dd", command: "dd if=/dev/zero of=/dev/sda bs=1M"},
		{name: "raw disk redirect", command: "echo x > /dev/sda"},
		{name: "fork bomb", command: `:(){ :|:& };:`},
		{name: "kill all processes", command: "kill -9 -1"},
		{name: "shutdown", command: "shutdown -h now"},
		{name: "reboot", command: "reboot"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			block, ok := checkHardlineCommand(tc.command)
			if !ok {
				t.Fatalf("checkHardlineCommand(%q) did not block", tc.command)
			}
			if block.Rule == "" || block.Reason == "" {
				t.Fatalf("block = %#v", block)
			}
		})
	}
}

func TestCheckHardlineCommandAllowsNonHardlineCommands(t *testing.T) {
	cases := []string{
		"echo ok",
		"go test ./tools/shell_command/...",
		"rm -rf ./tmp/generated",
		"chmod +x ./scripts/build.sh",
		"fail",
	}
	for _, command := range cases {
		t.Run(command, func(t *testing.T) {
			if block, ok := checkHardlineCommand(command); ok {
				t.Fatalf("checkHardlineCommand(%q) = %#v, true", command, block)
			}
		})
	}
}
