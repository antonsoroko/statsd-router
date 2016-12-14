// Work with statsdrouter config file

package statsdrouter

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
)

// Statsd node struct
type StatsdNode struct {
	Host           string `json:"host"`
	Port           uint16 `json:"port"`
	ManagementPort uint16 `json:"mgmt_port"`
}

// statsdrouter config file struct
type RouterConfig struct {
	Rules    map[string][]StatsdNode `json:"rules"`
	FilePath string                  `json:"-"`
}

// Creates a new config struct
// accepts a filepath to a config file as parameter
// returns the config struct and an error
func NewConfig(filepath string) (*RouterConfig, error) {
	if _, err := os.Stat(filepath); err != nil {
		if os.IsNotExist(err) {
			emptyConfig := RouterConfig{make(map[string][]StatsdNode), filepath}
			data, _ := json.MarshalIndent(emptyConfig, "", "  ")
			err = ioutil.WriteFile(filepath, data, 0644)
			if err != nil {
				log.Printf("Failed to write to file %s: %s", filepath, err)
				return nil, err
			}
		}
	}
	fileReader, err := os.Open(filepath)
	if err != nil {
		log.Printf("Failed to open file %s: %s", filepath, err)
		return nil, err
	}
	config, err := readConfigFile(fileReader)
	if err != nil {
		log.Printf("Failed to create new config from file %s: %s", filepath, err)
		return nil, err
	}
	config.FilePath = filepath
	return config, err
}

// Checks if StatsdNode is in []StatsdNode
func nodeInSlice(node StatsdNode, list []StatsdNode) bool {
	for _, v := range list {
		if v == node {
			return true
		}
	}
	return false
}

// Updates config and writes new config to the file
// accepts a *RouterConfig (recieved config) as parameter
// returns an error
func (config *RouterConfig) UpdateConfig(newConfig *RouterConfig) error {
	var err error
	for rule, nodes := range newConfig.Rules {
		if _, ok := config.Rules[rule]; !ok {
			config.Rules[rule] = nodes
		} else {
			for _, node := range newConfig.Rules[rule] {
				if inSlice := nodeInSlice(node, config.Rules[rule]); !inSlice {
					config.Rules[rule] = append(config.Rules[rule], node)
				}
			}
		}
	}
	jsonData, _ := json.MarshalIndent(config, "", "  ")
	err = ioutil.WriteFile(config.FilePath, jsonData, 0644)
	if err != nil {
		log.Printf("Failed to write to file %s: %s", config.FilePath, err)
		return err
	}
	return err
}

// Parses the raw json data into a RouterConfig struct
// accepts an io.Reader as parameter
// returns the RouterConfig struct and error
func readConfigFile(reader io.Reader) (*RouterConfig, error) {
	// TODO: check regexp (in case if we start with manually created config)
	var config RouterConfig
	raw_config, err := ioutil.ReadAll(reader)
	if err != nil {
		log.Printf("Failed to read from io.Reader: %s", err)
		return nil, err
	}
	err = json.Unmarshal(raw_config, &config)
	if err != nil {
		log.Printf("Failed to unmarshal config file: %s", err)
		return nil, err
	}
	return &config, nil
}

// Creates and returns new StatsdNode from string
// accepts a string like ip:port:mgmt_port
// returns the StatsdNode struct and error
// TODO: maybe this fuction is too complicated
func NewStatsdNode(hostString string) (statsdNode StatsdNode, err error) {
	hostComponents := strings.Split(hostString, ":")
	if len(hostComponents) != 3 {
		err = errors.New("hostString is invalid. Need 3 parts in format host:port:mgmt_port.")
		return
	}
	hostHostname := hostComponents[0]
	hostPort64, err := strconv.ParseUint(hostComponents[1], 10, 16)
	if err != nil {
		err = fmt.Errorf("Failed to convert master host port to int: %s", err)
		return
	}
	hostPort := uint16(hostPort64)
	hostManagementPort64, err := strconv.ParseUint(hostComponents[2], 10, 16)
	if err != nil {
		err = fmt.Errorf("Failed to convert master host management port to int: %s", err)
		return
	}
	hostManagementPort := uint16(hostManagementPort64)
	statsdNode = StatsdNode{hostHostname, hostPort, hostManagementPort}
	return
}
