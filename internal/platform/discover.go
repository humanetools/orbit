package platform

import "sync"

// DiscoveredService represents a service found on a connected platform.
type DiscoveredService struct {
	ID       string
	Name     string
	Platform string
}

// Discoverer is implemented by platforms that can list their services.
type Discoverer interface {
	DiscoverServices() ([]DiscoveredService, error)
}

// DiscoverAll runs service discovery concurrently across all given platforms.
// tokens maps platform name → decrypted API token.
// Returns all discovered services and a map of any per-platform errors.
func DiscoverAll(tokens map[string]string) ([]DiscoveredService, map[string]error) {
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		all      []DiscoveredService
		errMap   = make(map[string]error)
	)

	for name, token := range tokens {
		p, err := Get(name, token)
		if err != nil {
			errMap[name] = err
			continue
		}

		disc, ok := p.(Discoverer)
		if !ok {
			continue
		}

		wg.Add(1)
		go func(name string, disc Discoverer) {
			defer wg.Done()
			services, err := disc.DiscoverServices()
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errMap[name] = err
			} else {
				all = append(all, services...)
			}
		}(name, disc)
	}

	wg.Wait()
	return all, errMap
}
