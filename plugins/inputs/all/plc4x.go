//go:build !custom || inputs || inputs.plc4x

package all

import _ "github.com/influxdata/telegraf/plugins/inputs/plc4x" // register plugin