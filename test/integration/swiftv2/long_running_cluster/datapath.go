package long_running_cluster

import (
	"bytes"
	"fmt"
	"os/exec"
	"text/template"
)

func applyTemplate(templatePath string, data interface{}, kubeconfig string) error {
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "apply", "-f", "-")
	cmd.Stdin = &buf
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply failed: %s\n%s", err, string(out))
	}

	fmt.Println(string(out))
	return nil
}

// -------------------------
// PodNetwork
// -------------------------
type PodNetworkData struct {
	PNName      string
	VnetGUID    string
	SubnetGUID  string
	SubnetARMID string
	SubnetToken string
}

func CreatePodNetwork(kubeconfig string, data PodNetworkData, templatePath string) error {
	return applyTemplate(templatePath, data, kubeconfig)
}

// -------------------------
// PodNetworkInstance
// -------------------------
type PNIData struct {
	PNIName      string
	PNName       string
	Namespace    string
	Type         string
	Reservations int
}

func CreatePodNetworkInstance(kubeconfig string, data PNIData, templatePath string) error {
	return applyTemplate(templatePath, data, kubeconfig)
}

// -------------------------
// Pod
// -------------------------
type PodData struct {
	PodName  string
	NodeName string
	OS       string
	PNName   string
	PNIName  string
	Image    string
}

func CreatePod(kubeconfig string, data PodData, templatePath string) error {
	return applyTemplate(templatePath, data, kubeconfig)
}
