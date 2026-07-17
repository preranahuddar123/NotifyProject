package config

type Config struct {
	MySQLDetails struct {
		Username string `yaml:"username"`
		Password string `yaml:"address"`
		Address  string `yaml:"address"`
		Port     string `yaml:"port"`
		DBName   string `yaml:"db_name"`
	} `yaml:"mysql_details"`

	GrpcDetails struct {
		Network  string `yaml:"network"`
		Address  string `yaml:"address"`
		Endpoint string `yaml:"endpoint"`
	} `yaml:"grpc_details"`

	HttpDetails struct {
		Port string `yaml:"port"`
	} `yaml:"http_details"`
}
