//go:build load

package load

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"reflect"
	"runtime/debug"
	"strconv"
	"testing"

	"github.com/Azure/azure-container-networking/test/internal/kubernetes"
)

const (
	exitFail = 1

	// relative log directory
	logDir = "logs/"
)

var ErrInvalidVariableType = errors.New("variable type not supported")

func TestMain(m *testing.M) {
	var (
		err        error
		exitCode   int
		cnicleanup func() error
		cnscleanup func() error
	)

	LoadEnvironment(testConfig)

	defer func() {
		if r := recover(); r != nil {
			log.Println(string(debug.Stack()))
			exitCode = exitFail
		}

		if err != nil {
			log.Print(err)
			exitCode = exitFail
		} else {
			if cnicleanup != nil {
				cnicleanup()
			}
			if cnscleanup != nil {
				cnscleanup()
			}
		}

		os.Exit(exitCode)
	}()

	clientset, err := kubernetes.MustGetClientset()
	if err != nil {
		return
	}

	ctx := context.Background()
	installopt := os.Getenv(kubernetes.EnvInstallCNS)
	// create dirty cns ds
	if installCNS, err := strconv.ParseBool(installopt); err == nil && installCNS {
		if cnscleanup, err = kubernetes.InstallCNSDaemonset(ctx, clientset, logDir); err != nil {
			log.Print(err)
			exitCode = 2
			return
		}
	} else {
		log.Printf("Env %v not set to true, skipping", kubernetes.EnvInstallCNS)
	}

	exitCode = m.Run()
}

func LoadEnvironment(obj interface{}) {
	val := reflect.ValueOf(obj).Elem()
	typ := reflect.TypeOf(obj).Elem()

	for i := 0; i < val.NumField(); i++ {
		fieldVal := val.Field(i)
		fieldTyp := typ.Field(i)

		env := fieldTyp.Tag.Get("env")
		defaultVal := fieldTyp.Tag.Get("default")
		envVal := os.Getenv(env)

		if envVal == "" {
			envVal = defaultVal
		}

		switch fieldVal.Kind() {
		case reflect.Int:
			intVal, err := strconv.Atoi(envVal)
			if err != nil {
				panic(fmt.Sprintf("environment variable %q must be an integer", env))
			}
			fieldVal.SetInt(int64(intVal))
		case reflect.String:
			fieldVal.SetString(envVal)
		case reflect.Bool:
			boolVal, err := strconv.ParseBool(envVal)
			if err != nil {
				panic(fmt.Sprintf("environment variable %s must be a bool", env))
			}
			fieldVal.SetBool(boolVal)
		default:
			panic(ErrInvalidVariableType)
		}
	}
}
