package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type (
	Config struct {
		AppPort, CppContainerName, ContainerWorkingDir, CppSourceFileName, CppExecutableName, LogPath string
		CompilationTimeout, ExecutionTimeout                                                          time.Duration
	}
)

func NewConfig() *Config {
	_ = godotenv.Load()
	return &Config{
		LogPath:             getEnv("LOG_PATH", "app.log"),
		AppPort:             getEnv("APP_PORT", "7774"),
		CppContainerName:    getEnv("CPP_CONTAINER_NAME", "cpp-executor"),
		CppExecutableName:   getEnv("CPP_EXECUTABLE_NAME", "program"),
		CppSourceFileName:   getEnv("CPP_SOURCE_FILE_NAME", "main.cpp"),
		ContainerWorkingDir: getEnv("CONTAINER_WORKING_DIR", "/app"),
		CompilationTimeout:  getEnvTime("COMPILATION_TIME_OUT", 15),
		ExecutionTimeout:    getEnvTime("EXECUTION_TIME_OUT", 60),
	}
}

// getEnv returns the fallback value if the given key is not provided in env
func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvTime(key string, fallback int) time.Duration {
	if value := os.Getenv(key); value != "" {
		v, err := strconv.Atoi(value)
		if err == nil {
			return time.Duration(v) * time.Second
		}

	}
	return time.Duration(fallback) * time.Second
}

/*
cpp_container_name: "cpp-executor"
container_working_dir: "/app"
cpp_source_filename: "main.cpp"
cpp_executable_name: "program"
cpp_compilation_timeout: 15s
cpp_execution_timeout: 10s
*/
