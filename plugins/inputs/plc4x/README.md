# Siemens PLC4X Input Plugin

ThisPlugin for retrieving data from PLCs USE SDK PLC4X https://plc4x.apache.org/users/index.html

## Global configuration options <!-- @/docs/includes/plugin_config.md -->

In addition to the plugin-specific configuration settings, plugins support
additional global and plugin configuration settings. These settings are used to
modify metrics, tags, and field or create aliases and configure ordering, etc.
See the [CONFIGURATION.md][CONFIGURATION.md] for more details.

[CONFIGURATION.md]: ../../../docs/CONFIGURATION.md#plugins

## Configuration

```toml @sample.conf
# Plugin for retrieving data from PLCs USE SDK PLC4X https://plc4x.apache.org/users/index.html

[[inputs.plc4x]]
  ## Parameters to contact the PLC (mandatory)
  ## The schema is in "ads" , "bacnet-ip" , "c-bus" , "eip" , "knxnet-ip" ,"modbus-tcp" ,"opcua" , "s7"
   ## The domain_name is in the <host>[:port] format,the default port depends on the protocol ,ex. s7 is 102
  schema = "s7"
  domain_name = "127.0.0.1:102"
  
  ## Timeout for requests
  # timeout = "10s"

  ## Log detailed connection messages for debugging
  ## This option only has an effect when Telegraf runs in debug mode
  # debug_connection = false

  ## connect parameters depends on the protocel 
  ## the final connect url will be <schema>:<domain_name>?remote-slot=1&remote-rack=1
  # [[inputs.plc4x.parameters]]
  #   remote-slot = "1"
  #   remore-rack = "1"

  ## Metric definition(s)
  [[inputs.plc4x.metric]]
    ## Name of the measurement
    # name = "plc4x"

    ## Field definitions
    ## ex. s7
    fields = [
      { name="rpm",                  address="%DB1.DBD4:REAL"    },
      { name="status_ok",            address="%DB1.DBX2.1:BOOL"  },
      { name="status",               address="%DB1.DBW88:INT"  },
      { name="last_error_code",      address="%DB2.DBB34:BYTE" },
      { name="last_error_time",      address="%DB2.DBX10:DATE_AND_TIME"   }
    ]

    ## Tags assigned to the metric
    # [inputs.plc4x.metric.tags]
    #   device = "compressor"
    #   location = "main building"

```

## Example Output

```text
s7comm,host=Hugin rpm=712i,status_ok=true,last_error="empty slot",last_error_time=1611319681000000000i 1611332164000000000
```

## Metrics

The format of metrics produced by this plugin depends on the metric
configuration(s).
