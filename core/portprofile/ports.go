package portprofile

import (
	"fmt"
	"strconv"
)

const Field = "x-nymvpn-ports"
const maxPortsPerNode = 16
const maxCandidates = 128

func Expand(proxies, groups []map[string]any) ([]map[string]any, []map[string]any, error) {
	names := map[string]bool{"DIRECT": true, "REJECT": true, "GLOBAL": true}
	for _, items := range [][]map[string]any{proxies, groups} {
		for _, item := range items {
			name, _ := item["name"].(string)
			names[name] = true
		}
	}
	result := make([]map[string]any, 0, len(proxies))
	variants := map[string][]string{}
	count := 0
	for i, proxy := range proxies {
		value, enabled := proxy[Field]
		if !enabled {
			result = append(result, proxy)
			continue
		}
		name, ok := proxy["name"].(string)
		if !ok || name == "" {
			return nil, nil, fmt.Errorf("proxy %d: %s requires a name", i, Field)
		}
		switch proxy["type"] {
		case "vless", "vmess", "trojan", "ss", "hysteria2", "tuic":
		default:
			return nil, nil, fmt.Errorf("proxy %d: unsupported protocol for %s", i, Field)
		}
		ports, ok := value.([]any)
		if !ok || len(ports) > maxPortsPerNode {
			return nil, nil, fmt.Errorf("proxy %d: %s must contain at most %d integer ports", i, Field, maxPortsPerNode)
		}
		base := cloneMap(proxy)
		delete(base, Field)
		result = append(result, base)
		basePort, _ := portNumber(proxy["port"])
		seen := map[int]bool{basePort: true}
		for _, value := range ports {
			port, ok := portNumber(value)
			if !ok {
				return nil, nil, fmt.Errorf("proxy %d: %s accepts integer ports from 1 to 65535", i, Field)
			}
			if seen[port] {
				continue
			}
			seen[port] = true
			candidateName := name + " @" + strconv.Itoa(port)
			if names[candidateName] {
				return nil, nil, fmt.Errorf("proxy %d: generated port candidate name conflicts with an existing name", i)
			}
			count++
			if count > maxCandidates {
				return nil, nil, fmt.Errorf("%s allows at most %d extra candidates per profile", Field, maxCandidates)
			}
			names[candidateName] = true
			candidate := cloneMap(base)
			candidate["name"] = candidateName
			candidate["port"] = port
			delete(candidate, "ports")
			delete(candidate, "hop-interval")
			result = append(result, candidate)
			variants[name] = append(variants[name], candidateName)
		}
	}
	outputGroups := make([]map[string]any, 0, len(groups))
	for _, group := range groups {
		if group["type"] != "select" {
			outputGroups = append(outputGroups, group)
			continue
		}
		members, ok := group["proxies"].([]any)
		if !ok {
			outputGroups = append(outputGroups, group)
			continue
		}
		copy := cloneMap(group)
		expanded := make([]any, 0, len(members))
		for _, member := range members {
			expanded = append(expanded, member)
			name, _ := member.(string)
			for _, variant := range variants[name] {
				expanded = append(expanded, variant)
			}
		}
		copy["proxies"] = expanded
		outputGroups = append(outputGroups, copy)
	}
	return result, outputGroups, nil
}

func portNumber(value any) (int, bool) {
	var port int
	switch value := value.(type) {
	case int:
		port = value
	case float64:
		if value < 1 || value > 65535 || value != float64(int(value)) {
			return 0, false
		}
		port = int(value)
	default:
		return 0, false
	}
	return port, port >= 1 && port <= 65535
}

func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = cloneValue(value)
	}
	return output
}

func cloneValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		return cloneMap(value)
	case []any:
		output := make([]any, len(value))
		for i, item := range value {
			output[i] = cloneValue(item)
		}
		return output
	default:
		return value
	}
}
