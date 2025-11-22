package kubectl

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Client wraps kubectl operations
type Client struct {
	namespace string
}

// NewClient creates a new kubectl client
func NewClient(namespace string) *Client {
	return &Client{
		namespace: namespace,
	}
}

// GetPods returns a list of pod names for a given label selector
func (c *Client) GetPods(labelSelector string) ([]string, error) {
	cmd := exec.Command("kubectl", "get", "pods",
		"-n", c.namespace,
		"-l", labelSelector,
		"-o", "jsonpath={.items[*].metadata.name}")

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to get pods: %s", out.String())
	}

	podNames := strings.Fields(out.String())
	if len(podNames) == 0 {
		return nil, fmt.Errorf("no pods found with label %s", labelSelector)
	}

	return podNames, nil
}

// GetLogs retrieves logs from a pod
func (c *Client) GetLogs(podName string, follow bool, tail int) error {
	args := []string{"logs", "-n", c.namespace, podName}
	if follow {
		args = append(args, "-f")
	}
	if tail > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", tail))
	}

	cmd := exec.Command("kubectl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// GetEvents retrieves events for the namespace
func (c *Client) GetEvents() error {
	cmd := exec.Command("kubectl", "get", "events",
		"-n", c.namespace,
		"--sort-by", ".lastTimestamp")

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// PortForward forwards a local port to a pod port
func (c *Client) PortForward(podName string, localPort, remotePort int) error {
	portMapping := fmt.Sprintf("%d:%d", localPort, remotePort)
	cmd := exec.Command("kubectl", "port-forward",
		"-n", c.namespace,
		podName,
		portMapping)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// PortForwardService forwards a local port to a service port
func (c *Client) PortForwardService(serviceName string, localPort, remotePort int) error {
	portMapping := fmt.Sprintf("%d:%d", localPort, remotePort)
	cmd := exec.Command("kubectl", "port-forward",
		"-n", c.namespace,
		fmt.Sprintf("svc/%s", serviceName),
		portMapping)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// ExecCommand executes a command in a pod
func (c *Client) ExecCommand(podName string, command []string) error {
	args := []string{"exec", "-n", c.namespace, "-it", podName, "--"}
	args = append(args, command...)

	cmd := exec.Command("kubectl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

// GetService retrieves service information
func (c *Client) GetService(serviceName string) (string, error) {
	cmd := exec.Command("kubectl", "get", "svc",
		"-n", c.namespace,
		serviceName,
		"-o", "jsonpath={.metadata.name}")

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to get service: %s", out.String())
	}

	return out.String(), nil
}
