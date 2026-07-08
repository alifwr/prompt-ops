package docker

type ContainerInfo struct {
	ID      string   `json:"id"`
	Names   []string `json:"names"`
	Image   string   `json:"image"`
	State   string   `json:"state"`
	Status  string   `json:"status"`
	Ports   []string `json:"ports"`
	CPUUsage string   `json:"cpu_usage"`
	MemoryUsage string `json:"memory_usage"`
}
