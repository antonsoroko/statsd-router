// Map backends to rules
package statsdrouter

import (
	"fmt"
	"log"
	"regexp"
)

// Routing Map struct
type RoutingMap struct {
	Map map[string]*RoutingRule
	//internal fields:
	backendList   map[string]*StatsDBackend
	checkInterval int64
}

// Routing Rule struct
type RoutingRule struct {
	Regexp   *regexp.Regexp
	Backends []*StatsDBackend
}

// Creates a new RoutingMap struct
// accepts a checkInterval as parameter
// returns the *RoutingMap struct
func NewRoutingMap(checkInterval int64) *RoutingMap {
	result := RoutingMap{make(map[string]*RoutingRule), make(map[string]*StatsDBackend), checkInterval}
	return &result
}

// Updates RoutingMap
// accepts a *RouterConfig (recieved config) as parameter
// returns an error
func (routingMap *RoutingMap) UpdateRoutingMap(config *RouterConfig) error {
	var err error
	var needAdd bool
	for rule, nodes := range config.Rules {
		if _, ok := routingMap.Map[rule]; !ok {
			ruleRegexp := regexp.MustCompile(rule)
			routingMap.Map[rule] = &RoutingRule{Regexp: ruleRegexp}
		}
		for _, node := range nodes {
			needAdd = false
			backendKey := fmt.Sprintf("%s:%d:%d", node.Host, node.Port, node.ManagementPort)
			if _, ok := routingMap.backendList[backendKey]; !ok {
				if DebugMode {
					log.Printf("Creating new backend %s", backendKey)
				}
				routingMap.backendList[backendKey], err = NewStatsDBackend(node.Host, node.Port, node.ManagementPort, routingMap.checkInterval)
				if err != nil {
					log.Printf("Failed to Update RoutingMap with backend %s: %s", backendKey, err)
					return err
				}
				needAdd = true
			} else {
				if DebugMode {
					log.Printf("Using existing backend %s", backendKey)
				}
				backend := routingMap.backendList[backendKey]
				if inSlice := backendInSlice(backend, routingMap.Map[rule].Backends); !inSlice {
					needAdd = true
				}
			}
			if needAdd {
				backend := routingMap.backendList[backendKey]
				if DebugMode {
					log.Printf("Adding backend %s to rule %s", backendKey, rule)
				}
				routingMap.Map[rule].Backends = append(routingMap.Map[rule].Backends, backend)
			} else {
				if DebugMode {
					log.Printf("Backend %s already exist in rule %s", backendKey, rule)
				}
			}
		}
	}
	return err
}

// Checks if *StatsDBackend is in []*StatsDBackend
func backendInSlice(backend *StatsDBackend, list []*StatsDBackend) bool {
	for _, v := range list {
		if backend.Host == v.Host && backend.Port == v.Port && backend.ManagementPort == v.ManagementPort {
			return true
		}
	}
	return false
}
