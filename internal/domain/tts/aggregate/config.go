package aggregate

type Config struct {
	Provider      string  `json:"provider"`
	Voice         string  `json:"voice"`
	OutputDir     string  `json:"output_dir"`
	Format        string  `json:"format"`
	SampleRate    int     `json:"sample_rate"`
	Speed         float32 `json:"speed"`
	Pitch         float32 `json:"pitch"`
	Volume        float32 `json:"volume"`
	APIKey        string  `json:"api_key"`
	AppID         string  `json:"app_id"`
	Token         string  `json:"token"`
	Cluster       string  `json:"cluster"`
}