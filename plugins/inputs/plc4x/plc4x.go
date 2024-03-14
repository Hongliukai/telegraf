//go:generate ../../../tools/readme_config_includer/generator
package plc4x

import (
	"context"
	_ "embed"
	"errors"
	"fmt" //nolint:depguard // Required for tracing connection issues
	"strings"
	"time"

	plc4go "github.com/apache/plc4x/plc4go/pkg/api"
	drivers "github.com/apache/plc4x/plc4go/pkg/api/drivers"
	apiModel "github.com/apache/plc4x/plc4go/pkg/api/model"
	"github.com/apache/plc4x/plc4go/pkg/api/values"
	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/config"
	"github.com/influxdata/telegraf/metric"
	"github.com/influxdata/telegraf/plugins/inputs"
)

//go:embed sample.conf
var sampleConfig string

type metricFieldDefinition struct {
	Name    string `toml:"name"`
	Address string `toml:"address"`
	Mapping fieldMapping
}

type metricDefinition struct {
	Name   string                  `toml:"name"`
	Fields []metricFieldDefinition `toml:"fields"`
	Tags   map[string]string       `toml:"tags"`
}

type converterFunc func([]byte) interface{}

type fieldMapping struct {
	measurement string
	field       string
	tags        map[string]string
}

// Plc4x represents the plugin
type Plc4x struct {
	Schema     string             `toml:"schema"`
	DomainName string             `toml:"domain_name"`
	Parameters []map[string]string  `toml:"parameters"`
	Timeout    config.Duration    `toml:"timeout"`
	Configs    []metricDefinition `toml:"metric"`
	Log        telegraf.Logger    `toml:"-"`

	Url string
	DriverManager plc4go.PlcDriverManager 
	MetricFields  []metricFieldDefinition
	ReadRequest 	apiModel.PlcReadRequest
}

// SampleConfig returns a basic configuration for the plugin
func (*Plc4x) SampleConfig() string {
	return sampleConfig
}

// Init checks the config settings and prepares the plugin. It's called
// once by the Telegraf agent after parsing the config settings.
func (s *Plc4x) Init() error {
	// Check settings
	if s.Schema == "" {
		return errors.New("'schema' has to be specified")
	}
	if s.DomainName == "" {
		return errors.New("'domain_name' has to be specified")
	}
	if len(s.Configs) == 0 {
		return errors.New("no metric defined")
	}

	// Create a new instance of the PlcDriverManager
	driverManager := plc4go.NewPlcDriverManager()
	switch s.Schema {
	case "ads":
		drivers.RegisterAdsDriver(driverManager)
	case "bacnet-ip":
		drivers.RegisterBacnetDriver(driverManager)
	case "c-bus":
		drivers.RegisterBacnetDriver(driverManager)
	case "eip":
		drivers.RegisterEipDriver(driverManager)
	case "knxnet-ip":
		drivers.RegisterKnxDriver(driverManager)
	case "modbus-tcp" :
		drivers.RegisterModbusTcpDriver(driverManager)
	case "opcua":
		drivers.RegisterOpcuaDriver(driverManager)
	case "s7" :
		drivers.RegisterS7Driver(driverManager)
	default:
		return errors.New("unsupported protocol types")

	}
	s.DriverManager = driverManager
	url := fmt.Sprintf("%s://%s",s.Schema,s.DomainName)
	
	if s.Parameters !=nil && len(s.Parameters) > 0 {
		var parameterStrings []string
		for _,m1 := range s.Parameters {
			for key,value := range m1 {
				parameterStrings = append(parameterStrings,fmt.Sprintf("%s=%s"),key,value)
			}
		}
		url += "?" + strings.Join(parameterStrings,"&")
	}
	s.Url = url
	// Create the requests
	return s.createRequests()
}

// Start initializes the connection to the remote endpoint
func (s *Plc4x) Start(_ telegraf.Accumulator) error {
	s.Log.Debugf("Connecting to %q...", s.Url)
	crc := s.DriverManager.GetConnection(s.Url)
	// Wait for the driver to connect (or not)
	connectionResult := <-crc
	if connectionResult.GetErr() != nil {
		return fmt.Errorf("error connecting to PLC: %s",connectionResult.GetErr().Error())	
	}
	connection := connectionResult.GetConnection()
	if !connection.GetMetadata().CanRead() {
		return fmt.Errorf("This connection %s doesn't support read operations",s.Url)
	}
	// Prepare a read-request\
	readRequestBuilder :=connection.ReadRequestBuilder()
	for _,field := range s.MetricFields {
		readRequestBuilder.AddTagAddress(field.Name,field.Address)
	}
	readRequest, err := readRequestBuilder.Build()
	if err != nil {
		return fmt.Errorf("error preparing read-request: %s",connectionResult.GetErr().Error())	
	}
	s.ReadRequest = readRequest
	return nil
}

// Stop disconnects from the remote endpoint and cleans up
func (s *Plc4x) Stop() {
	if s.DriverManager != nil {
		s.Log.Debugf("Disconnecting from %q...", s.Url)
		s.DriverManager.Close()
	}
}

// Gather collects the data from the device
func (s *Plc4x) Gather(acc telegraf.Accumulator) error {
	timestamp := time.Now()
	grouper := metric.NewSeriesGrouper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(s.Timeout) )
	defer cancel()
	rrc := s.ReadRequest.ExecuteWithContext(ctx)
	select {
	case <- ctx.Done():
		return fmt.Errorf("read fields timeout")
	case rrr := <-rrc :
		if rrr.GetErr() != nil {
			s.Log.Errorf("error executing read-request: %s", rrr.GetErr().Error())
			s.Stop()
			return s.Start(acc)
		}
		for i, b := range s.MetricFields {
			// Read the batch
			s.Log.Debugf("Reading batch %d...", i+1)
			if rrr.GetResponse().GetResponseCode(b.Name) != apiModel.PlcResponseCode_OK {
				s.Log.Debugf("read tag %s error %s",b.Name,rrr.GetResponse().GetResponseCode(b.Name))
				continue
			}
			
			result := rrr.GetResponse().GetValue(b.Name)
	
			if value := convertSimpleValue(result) ; value != nil {
				grouper.Add(b.Mapping.measurement,b.Mapping.tags,timestamp,b.Mapping.field,value)
			} else {
				s.Log.Debugf(" tag %s value is unsupport %s",b.Name,result)
				continue
			}

	}
		
	}

	// Add the metrics grouped by series to the accumulator
	for _, x := range grouper.Metrics() {
		acc.AddMetric(x)
	}

	return nil
}

func convertSimpleValue(value values.PlcValue) interface{} {
	if value.IsNull() {
		return nil
	}
	//bit
	if value.IsBool() {
		return value.GetBool()
	}
	//int 
	if value.IsByte() {
		return value.GetByte()
	}
	if value.IsUint8() {
		return value.GetUint8()
	}
	if value.IsInt16() {
		return value.GetUint32()
	}
	if value.IsUint32() {
		return value.GetInt16()
	}
	if value.IsUint64() {
		return value.GetUint64()
	}
	if value.IsInt8() {
		return value.GetInt8()
	}
	if value.IsInt16() {
		return value.GetInt16()
	}
	if value.IsInt32() {
		return value.GetInt32()
	}
	if value.IsInt64() {
		return value.GetInt64()
	}
	//float
	if value.IsFloat32() {
		return value.GetFloat32()
	}
	if value.IsFloat64() {
		return value.GetFloat64()
	}
	//string
	if value.IsString() {
		return value.GetString()
	}
	//time
	if value.IsTime() {
		return value.GetTime()
	}
	if value.IsDuration() {
		return value.GetDuration()
	}
	if value.IsDate() {
		return value.GetDate()
	}
	if value.IsDateTime() {
		return value.GetDateTime()
	}

	return nil
}
// Internal functions
func (s *Plc4x) createRequests() error {
	checkFieldNameMap := make(map[string]metricFieldDefinition)

	for i, cfg := range s.Configs {
		// Set the defaults
		if cfg.Name == "" {
			cfg.Name = "plc4x"
		}

		// Check the metric definitions
		if len(cfg.Fields) == 0 {
			return fmt.Errorf("no fields defined for metric %q", cfg.Name)
		}

		// Create requests for all fields  and add it to the current slot
		for _, f := range cfg.Fields {
			if f.Name == "" {
				return fmt.Errorf("unnamed field in metric %q", cfg.Name)
			}

			m := fieldMapping{
				measurement: cfg.Name,
				field:       f.Name,
				tags:        cfg.Tags,
			}
			f.Mapping = m

			// Check for duplicate field definitions
			if _, exist := checkFieldNameMap[f.Name]; exist {
				return fmt.Errorf("duplicate field %q" , f.Name)
			}

			s.MetricFields = append(s.MetricFields, f)
		}

		// Update the configuration if changed
		s.Configs[i] = cfg
	}

	if s.MetricFields == nil || len(s.MetricFields) ==0 {
		return fmt.Errorf("no fields defined for metric %q")
	}

	return nil
}

// Add this plugin to telegraf
func init() {
	inputs.Add("plc4x", func() telegraf.Input {
		return &Plc4x{
			Timeout:      config.Duration(10 * time.Second),
		}
	})
}
