package discovery

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ayush-github123/podLen/pkg/models"
)

type Discoverer struct {
	kubeletURL   string
	apiServerURL string
	httpClient   *http.Client
	Cache        *PodCache
	bearerToken  string
	nodeName     string
}

type KubeletPod struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		UID       string `json:"uid"`
	} `json:"metadata"`
	Status struct {
		ContainerStatuses []struct {
			Name        string `json:"name"`
			ContainerID string `json:"containerID"`
		} `json:"containerStatuses"`
	} `json:"status"`
}

type KubeletPodsResponse struct {
	Items []KubeletPod `json:"items"`
}

func NewDiscoverer(kubeletURL string) *Discoverer {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				dialer := &net.Dialer{}
				return dialer.DialContext(ctx, network, addr)
			},
		},
		Timeout: 10 * time.Second,
	}

	// Read service account token
	tokenPath := "/var/run/secrets/kubernetes.io/serviceaccount/token"
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		fmt.Printf("Warning: Failed to read service account token: %v\n", err)
	}

	// Get node name from environment
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		nodeName = "localhost"
	}

	return &Discoverer{
		kubeletURL:   kubeletURL,
		apiServerURL: "https://kubernetes.default.svc",
		httpClient:   client,
		Cache:        NewPodCache(),
		bearerToken:  string(token),
		nodeName:     nodeName,
	}
}

func (d *Discoverer) DiscoverPods(ctx context.Context) ([]*models.Pod, error) {
	// Try using API server proxy first
	if d.bearerToken != "" {
		pods, err := d.discoverPodsViaAPIServer(ctx)
		if err == nil {
			return pods, nil
		}
		fmt.Printf("API server discovery failed (%v), falling back to direct kubelet access\n", err)
	}

	// Fallback to direct kubelet access
	return d.discoverPodsDirectly(ctx)
}

func (d *Discoverer) discoverPodsViaAPIServer(ctx context.Context) ([]*models.Pod, error) {
	// URL format: https://kubernetes.default.svc/api/v1/nodes/{node}/proxy/pods
	url := fmt.Sprintf("%s/api/v1/nodes/%s/proxy/pods", d.apiServerURL, d.nodeName)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add bearer token authentication
	if d.bearerToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", strings.TrimSpace(d.bearerToken)))
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query API server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API server returned status %d: %s", resp.StatusCode, string(body))
	}

	return d.parsePods(resp.Body)
}

func (d *Discoverer) discoverPodsDirectly(ctx context.Context) ([]*models.Pod, error) {
	url := fmt.Sprintf("%s/pods", d.kubeletURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add bearer token authentication
	if d.bearerToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", strings.TrimSpace(d.bearerToken)))
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query kubelet: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kubelet returned status %d: %s", resp.StatusCode, string(body))
	}

	return d.parsePods(resp.Body)
}

func (d *Discoverer) parsePods(body io.Reader) ([]*models.Pod, error) {
	var podsResponse KubeletPodsResponse
	if err := json.NewDecoder(body).Decode(&podsResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var pods []*models.Pod
	for _, kpod := range podsResponse.Items {
		pod := &models.Pod{
			Name:      kpod.Metadata.Name,
			Namespace: kpod.Metadata.Namespace,
			UID:       kpod.Metadata.UID,
			CreatedAt: time.Now(),
		}

		for _, containerStatus := range kpod.Status.ContainerStatuses {
			container := models.Container{
				Name:      containerStatus.Name,
				ID:        containerStatus.ContainerID,
				CreatedAt: time.Now(),
			}

			containerID := extractContainerID(containerStatus.ContainerID)
			container.CgroupID = containerID

			pod.Containers = append(pod.Containers, container)
		}

		pods = append(pods, pod)
	}

	return pods, nil
}

func (d *Discoverer) GetPods(ctx context.Context) ([]*models.Pod, error) {
	return d.Cache.GetAllPods(), nil
}

func (d *Discoverer) UpdateCache(pods []*models.Pod) {
	d.Cache.UpdatePods(pods)
}

func (d *Discoverer) GetChanges() (added []*models.Pod, removed []*models.Pod, updated []*models.Pod) {
	return d.Cache.GetChanges()
}

func (d *Discoverer) Run(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			pods, err := d.DiscoverPods(ctx)
			if err != nil {
				fmt.Printf("Error discovering pods: %v\n", err)
				continue
			}
			d.UpdateCache(pods)
		}
	}
}

func extractContainerID(fullID string) string {
	parts := bytes.Split([]byte(fullID), []byte("://"))
	if len(parts) > 1 {
		return string(parts[1])
	}
	return fullID
}
