// Provide API to work with rules

package statsdrouter

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

// JSON Error struct
type JsonError struct {
	Code    int    `json:"code"`
	Error   string `json:"error"`
	Message string `json:"message"`
}

// HTTP API struct
type HttpApi struct {
	port       uint16
	config     *RouterConfig
	routingMap *RoutingMap
}

// Creates and returns new HttpApi
// accepts a port, *RouterConfig and *RoutingMap
func NewHttpApi(port uint16, config *RouterConfig, routingMap *RoutingMap) *HttpApi {
	return &HttpApi{port: port, config: config, routingMap: routingMap}
}

// Endpoint to work with rules (list, add)
func (api *HttpApi) rules(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case "GET":
		jsonEnc := json.NewEncoder(w)
		jsonEnc.SetIndent("", "  ")
		jsonEnc.Encode(api.config)
		return
	case "POST":
		defer r.Body.Close()
		var err error
		newConfig, err := readConfigFile(r.Body)
		if err != nil {
			message, _ := json.Marshal(JsonError{Code: 500, Error: "failed to read incoming config", Message: err.Error()})
			http.Error(w, string(message), 500)
			return
		}
		err = api.routingMap.UpdateRoutingMap(newConfig)
		if err != nil {
			message, _ := json.Marshal(JsonError{Code: 500, Error: "failed to update routing map", Message: err.Error()})
			http.Error(w, string(message), 500)
			return
		}
		err = api.config.UpdateConfig(newConfig)
		if err != nil {
			message, _ := json.Marshal(JsonError{Code: 500, Error: "failed to update config", Message: err.Error()})
			http.Error(w, string(message), 500)
			return
		}
		log.Println(api.config)
		for k, v := range api.routingMap.Map {
			log.Println(k, v.Regexp, v.Backends)
		}
		json.NewEncoder(w).Encode(map[string]string{"message": "The config was successfully updated."})
		return
	default:
		message, _ := json.Marshal(JsonError{Code: 405, Error: "method not allowed"})
		http.Error(w, string(message), 405)
		return
	}
}

// Starts API's HTTP server
func (api *HttpApi) Start() {
	http.HandleFunc("/rules", api.rules)
	log.Printf("Starting API on port %d", api.port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", api.port), nil))
}
