package models

// ApplicationStatus 应用状态的数据传输对象
type ApplicationStatus struct {
	Name        string    `json:"name"`
	FilePath    string    `json:"file_path"`
	PID         int       `json:"pid"`
	Status      AppStatus `json:"status"`
	Uptime      string    `json:"uptime"`
	CPUUsage    string    `json:"cpu_usage"`
	MemoryUsage string    `json:"memory_usage"`
	MemorySize  string    `json:"memory_size"`
	LogFile     string    `json:"log_file"`
	ConfigFile  string    `json:"config_file"`
	Ports       string    `json:"ports"`
} 