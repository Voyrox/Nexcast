package main

import (
	"fmt"
	"log"

	tf "github.com/galeone/tensorflow/tensorflow/go"
	tg "github.com/galeone/tfgo"
)

const Lookback = 30

func main() {
	model := tg.LoadModel("model/demand_predictor", []string{"serve"}, nil)

	inputTensor, err := tf.NewTensor([1][Lookback][5]float32{})
	if err != nil {
		log.Fatalf("failed to create input tensor: %v", err)
	}

	// Fill the tensor data
	var sample [1][Lookback][5]float32
	for i := 0; i < Lookback; i++ {
		sample[0][i] = [5]float32{
			0,                     // system_id
			12,                    // deployed_nodes
			float32(i % 24),       // hour_of_day
			float32((i / 24) % 7), // day_of_week
			50.0,                  // demand
		}
	}

	inputTensor, err = tf.NewTensor(sample)
	if err != nil {
		log.Fatalf("failed to create input tensor: %v", err)
	}

	results := model.Exec(
		[]tf.Output{
			model.Op("StatefulPartitionedCall_1", 0),
		},
		map[tf.Output]*tf.Tensor{
			model.Op("serving_default_keras_tensor", 0): inputTensor,
		},
	)

	pred, ok := results[0].Value().([][]float32)
	if !ok {
		log.Fatalf("unexpected output type: %T", results[0].Value())
	}

	days := []string{
		"Monday", "Tuesday", "Wednesday", "Thursday",
		"Friday", "Saturday", "Sunday",
	}

	fmt.Println("Predicted demand:")
	for i, v := range pred[0] {
		if i < len(days) {
			fmt.Printf("%s: %.2f\n", days[i], v)
		}
	}
}
