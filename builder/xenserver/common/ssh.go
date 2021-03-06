package common

import (
	"bytes"
	"fmt"
	"github.com/mitchellh/multistep"
	"github.com/mitchellh/packer/communicator/ssh"
	"github.com/mitchellh/packer/packer"
	gossh "golang.org/x/crypto/ssh"
	commonssh "github.com/mitchellh/packer/common/ssh"
	"github.com/pkg/sftp"
	"io"
	"log"
	"net"
	"os"
	"strings"
)

func SSHAddress(state multistep.StateBag) (string, error) {
	sshIP := state.Get("ssh_address").(string)
	sshHostPort := 22
	return fmt.Sprintf("%s:%d", sshIP, sshHostPort), nil
}

func SSHLocalAddress(state multistep.StateBag) (string, error) {
	sshLocalPort, ok := state.Get("local_ssh_port").(uint)
	if !ok {
		return "", fmt.Errorf("SSH port forwarding hasn't been set up yet")
	}
	conn_str := fmt.Sprintf("%s:%d", "127.0.0.1", sshLocalPort)
	return conn_str, nil
}

func SSHPort(state multistep.StateBag) (int, error) {
	sshHostPort := state.Get("local_ssh_port").(uint)
	return int(sshHostPort), nil
}

func CommHost(state multistep.StateBag) (string, error) {
	return "127.0.0.1", nil
}

func SSHConfigFunc(config SSHConfig) func(multistep.StateBag) (*gossh.ClientConfig, error) {
	return func(state multistep.StateBag) (*gossh.ClientConfig, error) {
		config := state.Get("commonconfig").(CommonConfig)
		auth := []gossh.AuthMethod{
			gossh.Password(config.SSHPassword),
			gossh.KeyboardInteractive(
				ssh.PasswordKeyboardInteractive(config.SSHPassword)),
		}

		if config.SSHKeyPath != "" {
			signer, err := commonssh.FileSigner(config.SSHKeyPath)
			if err != nil {
				return nil, err
			}

			auth = append(auth, gossh.PublicKeys(signer))
		}

		return &gossh.ClientConfig{
			User: config.SSHUser,
			Auth: auth,
		}, nil
	}
}

func doExecuteSSHCmd(cmd, target string, config *gossh.ClientConfig) (stdout string, err error) {
	client, err := gossh.Dial("tcp", target, config)
	if err != nil {
		return "", err
	}

	//Create session
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}

	defer session.Close()

	var b bytes.Buffer
	session.Stdout = &b
	if err := session.Run(cmd); err != nil {
		return "", err
	}

	return strings.Trim(b.String(), "\n"), nil
}

func doExecuteSSHCmds(state multistep.StateBag, cmds[] string, target string, config *gossh.ClientConfig) (stdout string, err error) {
	ui := state.Get("ui").(packer.Ui)
	client, err := gossh.Dial("tcp", target, config)
	if err != nil {
		ui.Error(fmt.Sprintf("Error connecting to host. '%s'.", err))
		return "", err
	}

	var results bytes.Buffer

	// running command set
	for _, cmd := range cmds {
		//Create session for this command
		session, err := client.NewSession()
		if err != nil {
			ui.Error(fmt.Sprintf("Error creating SSH session on host. '%s'.", err))
			return "", err
		}

		defer session.Close()

		ui.Message(fmt.Sprintf("Running command: '%s'", cmd))
		var b bytes.Buffer
		var e bytes.Buffer
		session.Stdout = &b
		session.Stderr = &e
		// run this command
		if err := session.Run(cmd); err != nil {
			ui.Error(fmt.Sprintf("Error running SSH command '%s'.", err))
			ui.Error(fmt.Sprintf("stderr '%s'.", strings.Trim(e.String(), "\n")))
			return "", err
		}
	
		results.WriteString (b.String() + "\n")
	}

	return strings.Trim(results.String(), "\n"), nil
}

func ExecuteHostSSHCmds(state multistep.StateBag, cmds[] string) (stdout string, err error) {
	config := state.Get("commonconfig").(CommonConfig)

	// Setup connection config
	sshConfig := &gossh.ClientConfig{
		User: config.Username,
		Auth: []gossh.AuthMethod{
			gossh.Password(config.Password),
		},
	}
	return doExecuteSSHCmds(state, cmds, config.HostIp + ":22", sshConfig)
}


func ExecuteHostSSHCmd(state multistep.StateBag, cmd string) (stdout string, err error) {
	config := state.Get("commonconfig").(CommonConfig)
	// Setup connection config
	sshConfig := &gossh.ClientConfig{
		User: config.Username,
		Auth: []gossh.AuthMethod{
			gossh.Password(config.Password),
		},
	}
	return doExecuteSSHCmd(cmd, config.HostIp+":22", sshConfig)
}

func ExecuteGuestSSHCmd(state multistep.StateBag, cmd string) (stdout string, err error) {
	config := state.Get("commonconfig").(CommonConfig)
	localAddress, err := SSHLocalAddress(state)
	if err != nil {
		return
	}
	sshConfig, err := SSHConfigFunc(config.SSHConfig)(state)
	if err != nil {
		return
	}

	return doExecuteSSHCmd(cmd, localAddress, sshConfig)
}

func UploadFile (state multistep.StateBag, localFilename string, remoteFilename string, allowExecute bool) error {
	ui := state.Get("ui").(packer.Ui)
	config := state.Get("commonconfig").(CommonConfig)

	// Setup connection config
	sshConfig := &gossh.ClientConfig{
		User: config.Username,
		Auth: []gossh.AuthMethod{
			gossh.Password(config.Password),
		},
	}

	sshClient, err := gossh.Dial("tcp", config.HostIp + ":22", sshConfig)
	if err != nil {
		ui.Error(fmt.Sprintf("Error connecting to host. '%s'.", err))
		return err
	}
	defer sshClient.Close()

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		ui.Error(fmt.Sprintf("Error obtaining sftp client. '%s'.", err))
		return err
	}
	defer sftpClient.Close()

	sourceScript, err := os.Open(localFilename)
	if err != nil {
		ui.Error(fmt.Sprintf("Error opening local file '%s'. '%s'.", localFilename, err))
		return err
	}
	defer sourceScript.Close()

	destScript, err := sftpClient.Create(remoteFilename)
	if err != nil {
		ui.Error(fmt.Sprintf("Error creating file '%s' on remote host. '%s'.", remoteFilename, config.HostIp, err))
		return err
	}

	_, err = io.Copy(destScript, sourceScript)
	if err != nil {
		ui.Error(fmt.Sprintf("Error copying file '%s' to remote host. '%s'.", localFilename, config.HostIp, err))
		return err
	}

	if allowExecute {
		err := sftpClient.Chmod(remoteFilename, 555)
		if err != nil {
			ui.Error(fmt.Sprintf("Error setting permissions on file '%s' on remote host. '%s'.", remoteFilename, config.HostIp, err))
			return err
		}
	}

	return nil
}

func forward(local_conn net.Conn, config *gossh.ClientConfig, server, remote_dest string, remote_port uint) error {
	defer local_conn.Close()

	ssh_client_conn, err := gossh.Dial("tcp", server+":22", config)
	if err != nil {
		log.Printf("local ssh.Dial error: %s", err)
		return err
	}
	defer ssh_client_conn.Close()

	remote_loc := fmt.Sprintf("%s:%d", remote_dest, remote_port)
	ssh_conn, err := ssh_client_conn.Dial("tcp", remote_loc)
	if err != nil {
		log.Printf("ssh.Dial error: %s", err)
		return err
	}
	defer ssh_conn.Close()

	txDone := make(chan struct{})
	rxDone := make(chan struct{})

	go func() {
		_, err = io.Copy(ssh_conn, local_conn)
		if err != nil {
			log.Printf("io.copy failed: %v", err)
		}
		close(txDone)
	}()

	go func() {
		_, err = io.Copy(local_conn, ssh_conn)
		if err != nil {
			log.Printf("io.copy failed: %v", err)
		}
		close(rxDone)
	}()

	<-txDone
	<-rxDone

	return nil
}

func ssh_port_forward(local_listener net.Listener, remote_port uint, remote_dest, host, username, password string) error {

	config := &gossh.ClientConfig{
		User: username,
		Auth: []gossh.AuthMethod{
			gossh.Password(password),
		},
	}

	for {
		local_connection, err := local_listener.Accept()

		if err != nil {
			log.Printf("Local accept failed: %s", err)
			return err
		}

		// Forward to a remote port
		go forward(local_connection, config, host, remote_dest, remote_port)
	}

	return nil
}
