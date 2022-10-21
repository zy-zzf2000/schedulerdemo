package plugin

import (
	"ouo-scheduler/pkg/plugin/ouo"

	"github.com/spf13/cobra"
	"k8s.io/kubernetes/cmd/kube-scheduler/app"
)

// Register ... register a new scheduler
func Register() *cobra.Command {
	return app.NewSchedulerCommand(
		app.WithPlugin(ouo.Name, ouo.New),
	)
}
