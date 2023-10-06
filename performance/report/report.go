package report

type ActionMetric struct {
	PodMetric map[string]Metric `json:"pod_metric"`
}

type Metric struct {
	FileName  string `json:"file_name"`
	MaxCpu    int    `json:"max_cpu"`
	MaxMemory int    `json:"max_memory"`
	MinCpu    int    `json:"min_cpu"`
	MinMemory int    `json:"min_memory"`
}
type ScheduelTime struct {
	MaxTime int `json:"max_time"`
	MinTime int `json:"min_time"`
}

type Report struct {
	ClusterCount         int
	PlacementCount       int
	TotalApiRequestCount map[string]int          `json:"max_cur_hour_api_request_count"`
	AllMetrics           map[string]ActionMetric `json:"all_metrics"`
	PlacementSchedule    map[string]ScheduelTime `json:"placement_schedule"`
}

func NewReport() *Report {
	return &Report{
		TotalApiRequestCount: make(map[string]int),
		AllMetrics:           make(map[string]ActionMetric),
		PlacementSchedule:    make(map[string]ScheduelTime),
	}
}
