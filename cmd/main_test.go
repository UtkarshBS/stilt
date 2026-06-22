package main

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSparkUsesPinnedApacheImage(t *testing.T) {
	data, err := os.ReadFile("../config/services.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Services map[string]ServiceConfig `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}

	master := config.Services["spark-master"]
	if master.Image != "apache/spark" || master.Version != "3.5.8" {
		t.Fatalf("unexpected Spark master image: %s:%s", master.Image, master.Version)
	}
	if master.Command != "/opt/spark/bin/spark-class org.apache.spark.deploy.master.Master --host spark-master --port 7077 --webui-port 8080\n" {
		t.Fatalf("unexpected Spark master command: %q", master.Command)
	}

	worker := config.Services["spark-worker"]
	if worker.Image != master.Image || worker.Version != master.Version {
		t.Fatalf("Spark worker image does not match master: %s:%s", worker.Image, worker.Version)
	}
	if worker.Command != "/opt/spark/bin/spark-class org.apache.spark.deploy.worker.Worker --webui-port 8081 spark://spark-master:7077\n" {
		t.Fatalf("unexpected Spark worker command: %q", worker.Command)
	}
}

func TestBuildComposeCarriesDependsOnAndEnv(t *testing.T) {
	services := map[string]ServiceConfig{
		"kafka": {
			Image:     "confluentinc/cp-kafka",
			Version:   "7.3.0",
			DependsOn: []string{"zookeeper"},
		},
		"flink-jobmanager": {
			Image:   "apache/flink",
			Version: "2.2.0-scala_2.12",
			Command: "jobmanager",
			Environment: map[string]string{
				"FLINK_PROPERTIES": "jobmanager.rpc.address: flink-jobmanager",
			},
		},
		"flink-taskmanager": {
			Image:     "apache/flink",
			Version:   "2.2.0-scala_2.12",
			Command:   "taskmanager",
			DependsOn: []string{"flink-jobmanager"},
			Environment: map[string]string{
				"FLINK_PROPERTIES": "jobmanager.rpc.address: flink-jobmanager\ntaskmanager.numberOfTaskSlots: 2",
			},
		},
	}

	compose := buildCompose(services, map[string]string{})

	kafka := compose.Services["kafka"]
	if len(kafka.DependsOn) != 1 || kafka.DependsOn[0] != "zookeeper" {
		t.Fatalf("unexpected kafka depends_on: %#v", kafka.DependsOn)
	}

	jobmanager := compose.Services["flink-jobmanager"]
	if got := jobmanager.Environment["FLINK_PROPERTIES"]; got != "jobmanager.rpc.address: flink-jobmanager" {
		t.Fatalf("unexpected flink jobmanager env: %q", got)
	}

	taskmanager := compose.Services["flink-taskmanager"]
	if len(taskmanager.DependsOn) != 1 || taskmanager.DependsOn[0] != "flink-jobmanager" {
		t.Fatalf("unexpected flink taskmanager depends_on: %#v", taskmanager.DependsOn)
	}

	if got := taskmanager.Environment["FLINK_PROPERTIES"]; got != "jobmanager.rpc.address: flink-jobmanager\ntaskmanager.numberOfTaskSlots: 2" {
		t.Fatalf("unexpected flink env: %q", got)
	}
}

func TestBuildComposeResolvesGeneratedSecrets(t *testing.T) {
	compose := buildCompose(map[string]ServiceConfig{
		"postgres": {
			Image:   "postgres",
			Version: "16-alpine",
			Environment: map[string]string{
				"POSTGRES_PASSWORD": "{{GENERATE_24}}",
			},
		},
	}, map[string]string{
		"POSTGRES_PASSWORD": "secret-value",
	})

	if got := compose.Services["postgres"].Environment["POSTGRES_PASSWORD"]; got != "secret-value" {
		t.Fatalf("unexpected secret resolution: %q", got)
	}
}

func TestDockerBuildArgs(t *testing.T) {
	build := &BuildConfig{
		Context:          ".",
		DockerfileInline: "FROM apache/flink:${FLINK_VERSION}\nRUN echo ready\n",
		Args: map[string]string{
			"FLINK_VERSION": "2.2.0-scala_2.12",
		},
	}
	got := dockerBuildArgs("stilt-flink:2.2.0-scala_2.12", *build)
	want := []string{
		"build",
		"--build-arg", "FLINK_VERSION=2.2.0-scala_2.12",
		"-t", "stilt-flink:2.2.0-scala_2.12",
		"-f", "-", ".",
	}
	if len(got) != len(want) {
		t.Fatalf("unexpected build args: %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected build args: %#v", got)
		}
	}
}
