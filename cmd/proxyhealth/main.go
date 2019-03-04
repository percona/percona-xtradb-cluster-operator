package main

import (
	"fmt"
	"github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/pxc/app/proxysqlcnf"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
)

var (
	connstr string
)

var rootCmd = &cobra.Command{
	Use:   "proxyhealth",
	Short: "Liveness and Readiness probe for ProxySQL",
}

var livenessCmd = &cobra.Command{
	Use:   "liveness",
	Short: "Liveness probe for ProxySQL",
	Long:  `Tries to connect to database. Pass if connection is open. Fail if can't connect.'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := proxysqlcnf.LivenessProbe(connstr); err != nil {
			return errors.Wrap(err, "liveness probe has failed")
		}
		return nil
	},
}

var readinessCmd = &cobra.Command{
	Use:   "readiness",
	Short: "Readiness probe for ProxySQL",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := proxysqlcnf.ReadinessProbe(connstr); err != nil {
			return errors.Wrap(err, "readiness probe has failed")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(livenessCmd, readinessCmd)
	rootCmd.PersistentFlags().StringVar(&connstr,
		"connstr",
		"username:password@tcp(127.0.0.1:3306)/test",
		"Set the connection string for database",
	)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
