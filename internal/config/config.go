package config

import (
	"os"
	"encoding/json"
)

const configFileName = ".gatorconfig.json"

type Config struct {
	DBUrl string `json:"db_url"`
	Current_user_name string `json:"current_user_name"`

}

func Read() (Config, error) {
	file, err := getConfigFilePath()
	if err != nil {
		return Config{}, err
	}
	content, err := os.ReadFile(file)
	if err != nil {
		return Config{}, err
	}
	var con Config
	if err = json.Unmarshal(content, &con); err != nil {
		return Config{}, err
	}
	return con, nil
}

func getConfigFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return home + "/" + configFileName, nil
}


func (c Config) SetUser(user string) error {
	c.Current_user_name = user
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	path, err := getConfigFilePath()
	if err != nil {
		return err
	}
	if err = os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	return nil
}



