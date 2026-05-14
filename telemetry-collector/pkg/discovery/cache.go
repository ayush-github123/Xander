package discovery

import (
	"sync"

	"github.com/ayush-github123/podLen/pkg/models"
)

type PodCache struct {
	mu       sync.RWMutex
	pods     map[string]*models.PodState
	lastSync map[string]*models.Pod
}

func NewPodCache() *PodCache {
	return &PodCache{
		pods:     make(map[string]*models.PodState),
		lastSync: make(map[string]*models.Pod),
	}
}

func (pc *PodCache) UpdatePods(newPods []*models.Pod) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	currentUIDs := make(map[string]bool)
	for _, pod := range newPods {
		currentUIDs[pod.UID] = true

		if state, exists := pc.pods[pod.UID]; exists {
			state.Pod = *pod
			state.LastUpdated = pod.CreatedAt
		} else {
			state := &models.PodState{
				Pod:         *pod,
				Metrics:     make(map[string]*models.Metrics),
				LastUpdated: pod.CreatedAt,
			}
			pc.pods[pod.UID] = state
		}

		pc.lastSync[pod.UID] = pod
	}

	for uid := range pc.pods {
		if !currentUIDs[uid] {
			delete(pc.pods, uid)
			delete(pc.lastSync, uid)
		}
	}
}

func (pc *PodCache) GetAllPods() []*models.Pod {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	var pods []*models.Pod
	for _, state := range pc.pods {
		pod := state.Pod
		pods = append(pods, &pod)
	}
	return pods
}

func (pc *PodCache) GetPodByUID(uid string) *models.Pod {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	if state, exists := pc.pods[uid]; exists {
		pod := state.Pod
		return &pod
	}
	return nil
}

func (pc *PodCache) GetPodState(uid string) *models.PodState {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	if state, exists := pc.pods[uid]; exists {
		stateCopy := *state
		return &stateCopy
	}
	return nil
}

func (pc *PodCache) UpdatePodMetrics(podUID string, containerID string, metrics *models.Metrics) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if state, exists := pc.pods[podUID]; exists {
		state.Metrics[containerID] = metrics
	}
}

func (pc *PodCache) GetChanges() (added []*models.Pod, removed []*models.Pod, updated []*models.Pod) {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	currentUIDs := make(map[string]bool)
	for _, state := range pc.pods {
		currentUIDs[state.Pod.UID] = true
	}

	for _, state := range pc.pods {
		if _, existed := pc.lastSync[state.Pod.UID]; !existed {
			pod := state.Pod
			added = append(added, &pod)
		}
	}

	for uid, oldPod := range pc.lastSync {
		if !currentUIDs[uid] {
			removed = append(removed, oldPod)
		}
	}

	return added, removed, updated
}

func (pc *PodCache) Clear() {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.pods = make(map[string]*models.PodState)
	pc.lastSync = make(map[string]*models.Pod)
}

func (pc *PodCache) Count() int {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	return len(pc.pods)
}
