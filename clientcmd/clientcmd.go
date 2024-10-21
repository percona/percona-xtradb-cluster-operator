package clientcmd

import (
	"bufio"
	"context"
	"io"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/retry"
)

type Client struct {
	client     corev1client.CoreV1Interface
	restconfig *restclient.Config
}

func NewClient() (*Client, error) {
	// Instantiate loader for kubeconfig file.
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{
			Timeout: "10s",
		},
	)

	// Get a rest.Config from the kubeconfig file.  This will be passed into all
	// the client objects we create.
	restconfig, err := kubeconfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	// Create a Kubernetes core/v1 client.
	cl, err := corev1client.NewForConfig(restconfig)
	if err != nil {
		return nil, err
	}

	return &Client{
		client:     cl,
		restconfig: restconfig,
	}, nil
}

func (c *Client) PodLogs(namespace, podName string, opts *corev1.PodLogOptions) ([]string, error) {
	var logArr []string
	retryErr := retry.OnError(retry.DefaultRetry, func(err error) bool {
		return true // Retry on all errors
	}, func() error {
		logs, err := c.client.Pods(namespace).GetLogs(podName, opts).Stream(context.TODO())
		if err != nil {
			return errors.Wrap(err, "get pod logs stream")
		}
		defer logs.Close()
		logArr = make([]string, 0)
		sc := bufio.NewScanner(logs)
		for sc.Scan() {
			logArr = append(logArr, sc.Text())
		}
		if sc.Err() != nil {
			return errors.Wrap(sc.Err(), "reading logs stream")
		}
		return nil
	})
	if retryErr != nil {
		return nil, errors.Wrap(retryErr, "failed to get pod logs")
	}
	return logArr, nil
}

func (c *Client) IsPodRunning(namespace, podName string) (bool, error) {
	var isRunning bool
	retryErr := retry.OnError(retry.DefaultRetry, func(err error) bool {
		return true // Retry on all errors
	}, func() error {
		pod, err := c.client.Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if pod.Status.Phase != corev1.PodRunning {
			isRunning = false
			return nil
		}
		for _, v := range pod.Status.Conditions {
			if v.Type == corev1.ContainersReady && v.Status == corev1.ConditionTrue {
				isRunning = true
				return nil
			}
		}
		isRunning = false
		return nil
	})
	if retryErr != nil {
		return false, errors.Wrap(retryErr, "failed to check pod status")
	}
	return isRunning, nil
}

func (c *Client) Exec(pod *corev1.Pod, containerName string, command []string, stdin io.Reader, stdout, stderr io.Writer, tty bool) error {
	// Prepare the API URL used to execute another process within the Pod.  In
	// this case, we'll run a remote shell.
	req := c.client.RESTClient().
		Post().
		Namespace(pod.Namespace).
		Resource("pods").
		Name(pod.Name).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   command,
			Stdin:     stdin != nil,
			Stdout:    stdout != nil,
			Stderr:    stderr != nil,
			TTY:       tty,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(c.restconfig, "POST", req.URL())
	if err != nil {
		return errors.Wrap(err, "failed to create executor")
	}

	retryErr := retry.OnError(retry.DefaultRetry, func(err error) bool {
		return true // Retry on all errors
	}, func() error {
		return exec.Stream(remotecommand.StreamOptions{
			Stdin:  stdin,
			Stdout: stdout,
			Stderr: stderr,
			Tty:    tty,
		})
	})

	if retryErr != nil {
		return errors.Wrap(retryErr, "failed to execute command in pod")
	}

	return nil
}

func (c *Client) REST() restclient.Interface {
	return c.client.RESTClient()
}
