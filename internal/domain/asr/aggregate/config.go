package aggregate

type Config struct {
	Provider      string  `json:"provider"`
	AppID         string  `json:"appid"`
	AccessToken   string  `json:"access_token"`
	OutputDir     string  `json:"output_dir"`
	Language      string  `json:"language"`
	SampleRate    int     `json:"sample_rate"`
	Format        string  `json:"format"`
	EnablePunctuation bool `json:"enable_punctuation"`
	EnableITN      bool   `json:"enable_itn"`
}