//go:build android

package anet

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/base/utils"
)

var (
	mu           sync.RWMutex
	cachedIfaces []net.Interface
	cachedAddrs  []net.Addr
	perIface     map[string][]net.Addr

	providerMu sync.RWMutex
	provider   Provider

	notifierMu sync.RWMutex
	notifier   Notifier

	errInterfaceNotFound = errors.New("interface not found")
)

func buildFromList(list []NetworkInterface) ([]net.Interface, []net.Addr, map[string][]net.Addr) {
	var ifaces []net.Interface
	var addrs []net.Addr
	m := make(map[string][]net.Addr, len(list))
	for i := range list {
		iface, ifaceAddrs := list[i].ToNetInterface()
		if iface.Index == 0 {
			iface.Index = len(ifaces) + 1
		}
		ifaces = append(ifaces, iface)
		m[iface.Name] = ifaceAddrs
		addrs = append(addrs, ifaceAddrs...)
	}
	return ifaces, addrs, m
}

func updateCache(list []NetworkInterface) ([]net.Interface, []net.Addr) {
	ifaces, addrs, m := buildFromList(list)
	mu.Lock()
	cachedIfaces = ifaces
	cachedAddrs = addrs
	perIface = m
	mu.Unlock()
	return ifaces, addrs
}

func truncateForLog(s string) string {
	if len(s) > 1024 {
		return s[:1024] + "..."
	}
	return s
}

func RegisterProvider(p Provider) {
	providerMu.Lock()
	defer providerMu.Unlock()
	if utils.IsNilCallback(p) {
		provider = nil
		return
	}
	provider = p
}

func HasProvider() bool {
	providerMu.RLock()
	defer providerMu.RUnlock()
	return !utils.IsNilCallback(provider)
}

func RegisterNotifier(n Notifier) {
	notifierMu.Lock()
	defer notifierMu.Unlock()
	if utils.IsNilCallback(n) {
		notifier = nil
		return
	}
	notifier = n
}

func HasNotifier() bool {
	notifierMu.RLock()
	defer notifierMu.RUnlock()
	return !utils.IsNilCallback(notifier)
}

func TriggerNetworkChange() {
	notifierMu.RLock()
	n := notifier
	notifierMu.RUnlock()
	if !utils.IsNilCallback(n) {
		n.OnNetworkChange()
	}
}

func UpdateInterfaces(list []NetworkInterface) {
	updateCache(list)

	notifierMu.RLock()
	n := notifier
	notifierMu.RUnlock()
	if !utils.IsNilCallback(n) {
		n.OnNetworkChange()
	}
}

func UpdateInterfacesJSON(jsonStr string) error {
	var list []NetworkInterface
	err := json.Unmarshal([]byte(jsonStr), &list)
	if err != nil {
		log.Warningf("anet: failed to parse interfaces JSON: %s", err)
		return err
	}
	UpdateInterfaces(list)
	return nil
}

func Interfaces() ([]net.Interface, error) {
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if !utils.IsNilCallback(p) {
		jsonStr := p.GetInterfacesJSON()
		var list []NetworkInterface
		err := json.Unmarshal([]byte(jsonStr), &list)
		if err == nil {
			ifaces, _ := updateCache(list)
			out := make([]net.Interface, len(ifaces))
			copy(out, ifaces)
			return out, nil
		}
		log.Warningf("anet: failed to parse provider interfaces JSON: %s", truncateForLog(jsonStr))
	}

	mu.RLock()
	if cachedIfaces != nil {
		out := make([]net.Interface, len(cachedIfaces))
		copy(out, cachedIfaces)
		mu.RUnlock()
		return out, nil
	}
	mu.RUnlock()
	return net.Interfaces()
}

func InterfaceAddrs() ([]net.Addr, error) {
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if !utils.IsNilCallback(p) {
		if _, err := Interfaces(); err == nil {
			mu.RLock()
			if cachedAddrs != nil {
				out := make([]net.Addr, len(cachedAddrs))
				copy(out, cachedAddrs)
				mu.RUnlock()
				return out, nil
			}
			mu.RUnlock()
		}
	}

	mu.RLock()
	if cachedAddrs != nil {
		out := make([]net.Addr, len(cachedAddrs))
		copy(out, cachedAddrs)
		mu.RUnlock()
		return out, nil
	}
	mu.RUnlock()
	return net.InterfaceAddrs()
}

func AddrsForInterface(name string) ([]net.Addr, error) {
	// If Provider is registered but cache not yet populated, sync from it.
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if !utils.IsNilCallback(p) {
		mu.RLock()
		hasCache := perIface != nil
		mu.RUnlock()
		if !hasCache {
			// Populate cache via provider.
			if _, err := Interfaces(); err != nil {
				log.Tracef("anet: failed to sync interface cache: %s", err)
			}
		}
	}

	mu.RLock()
	if perIface != nil {
		if addrs, ok := perIface[name]; ok {
			out := make([]net.Addr, len(addrs))
			copy(out, addrs)
			mu.RUnlock()
			return out, nil
		}
		mu.RUnlock()
		return nil, fmt.Errorf("%w: %q", errInterfaceNotFound, name)
	}
	mu.RUnlock()

	// Before first push, fall back to OS.
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, err
	}
	return iface.Addrs()
}
