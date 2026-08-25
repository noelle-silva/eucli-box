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

func TestCheckHardlineCommandBlocksAddedAbsoluteDestructionCommands(t *testing.T) {
	cases := []struct {
		name    string
		command string
	}{
		{name: "powershell critical path delete", command: "Remove-Item -Path 'C:\\Windows' -Recurse -Force"},
		{name: "powershell wrapped critical path delete", command: "powershell -Command \"Remove-Item -Path 'C:\\Windows' -Recurse -Force\""},
		{name: "split recursive force critical path delete", command: "rm -r -f /etc"},
		{name: "shell wrapped recursive force critical path delete", command: "bash -lc \"rm -r -f /etc\""},
		{name: "sudo recursive force critical path delete", command: "sudo rm -rf /etc"},
		{name: "sudo user recursive force critical path delete", command: "sudo -u root rm -rf /etc"},
		{name: "trap recursive force critical path delete", command: "trap 'rm -rf /etc' EXIT"},
		{name: "nohup recursive force critical path delete", command: "nohup rm -rf /etc"},
		{name: "cmd recursive force critical path delete", command: "del /s /f C:\\Windows\\*"},
		{name: "cmd wrapped critical path delete", command: "cmd /c \"del /s /f C:\\Windows\\*\""},
		{name: "sudo shutdown", command: "sudo shutdown -h now"},
		{name: "sudo systemctl reboot", command: "sudo systemctl reboot"},
		{name: "systemctl poweroff", command: "systemctl --force poweroff"},
		{name: "init halt", command: "init 0"},
		{name: "telinit reboot", command: "telinit 6"},
		{name: "shred disk", command: "shred -z /dev/sda"},
		{name: "wipe filesystem signatures", command: "wipefs --all /dev/nvme0n1"},
		{name: "recreate partition table", command: "parted --script /dev/sda mklabel gpt"},
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
		"Remove-Item -Path C:\\Temp -Recurse -Force",
		"Remove-Item -Path C:\\Temp -Recurse:$false -Force",
		"Remove-Item -Path C:\\Temp -Recurse -Force:$false",
		"sudo rm -rf ./tmp/generated",
		"nohup rm -rf ./tmp/generated",
		"del /s /f C:\\Temp\\*",
		"systemctl status reboot",
		"systemctl --user reboot",
		"init 1",
		"shred ./important.txt",
		"wipefs /dev/sda",
		"parted --script /dev/sda print",
		"diskpart /s cleanup.txt",
	}
	for _, command := range cases {
		t.Run(command, func(t *testing.T) {
			if block, ok := checkHardlineCommand(command); ok {
				t.Fatalf("checkHardlineCommand(%q) = %#v, true", command, block)
			}
		})
	}
}
