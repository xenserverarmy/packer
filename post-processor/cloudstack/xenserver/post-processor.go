package xenserver

import (
	"errors"
	"fmt"
	"github.com/mitchellh/packer/common"
	"github.com/mitchellh/packer/helper/config"
	"github.com/mitchellh/packer/packer"
	"github.com/mitchellh/packer/template/interpolate"
	"strings"
	"github.com/svanharmelen/gocs"
	"time"
	"os"
	"io"
	"path"
	"encoding/json"
	"compress/gzip"
)

var builtins = map[string]string{
	"packer.xenserver": "xenserver",
}

type Config struct {
	common.PackerConfig `mapstructure:",squash"`

	ApiUrl    string `mapstructure:"apiurl"`
	ApiKey    string `mapstructure:"apikey"`
	Secret    string `mapstructure:"secret"`

	DisplayText   string `mapstructure:"display_text"`
	TemplateName	string `mapstructure:"template_name"`
	OsType		string `mapstructure:"os_type"`
	DownloadUrl	string `mapstructure:"download_url"`
	Zone		string `mapstructure:"zone"`
	Account	string `mapstructure:"account"`
	Domain		string `mapstructure:"domain"`
	PwdEnabled	bool `mapstructure:"password_enabled"`
	SshEnabled	bool `mapstructure:"ssh_enabled"`
	HasTools	bool `mapstructure:"has_tools"` // isdynamicallyscalable in CloudStack vernacular
	UploadTimer	uint `mapstructure:"upload_timer"`
	CompressVhd	bool `mapstructure:"compress_vhd"`

	ctx interpolate.Context
}

type TemplateResponse struct {
	IsReady	bool
	Status		string
}

type PostProcessor struct {
	config Config
}

func (p *PostProcessor) Configure(raws ...interface{}) error {

	err := config.Decode(&p.config, &config.DecodeOpts{
		Interpolate:        true,
		InterpolateContext: &p.config.ctx,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{},
		},
	}, raws...)
	if err != nil {
		return err
	}

	if p.config.UploadTimer == 0 {
		p.config.UploadTimer  = 1200
	}

	// Accumulate any errors
	errs := new(packer.MultiError)

	// First define all our templatable parameters that are _required_
	templates := map[string]*string{
		"apiurl":     &p.config.ApiUrl,
		"apikey":    	&p.config.ApiKey,
		"secret":     &p.config.Secret,
		"display_text":   &p.config.DisplayText,
		"template_name":  &p.config.TemplateName,
		"os_type":    &p.config.OsType,
		"download_url":   &p.config.DownloadUrl,
		"zone":       &p.config.Zone,
	}

	for key, ptr := range templates {
		if *ptr == "" {
			errs = packer.MultiErrorAppend(
				errs, fmt.Errorf("%s must be set", key))
		}
	}

	// Then define the ones that are optional
	if p.config.Account != "" && p.config.Domain == "" {
		errs = packer.MultiErrorAppend(
			errs, errors.New("If the account is specified, the domain must also be specified."))
	}


	if len(errs.Errors) > 0 {
		return errs
	}

	return nil
}

func (p *PostProcessor) PostProcess(ui packer.Ui, artifact packer.Artifact) (packer.Artifact, bool, error) {
	if _, ok := builtins[artifact.BuilderId()]; !ok {
		return nil, false, fmt.Errorf("Unknown artifact type, can't process artifact: %s", artifact.BuilderId())
	}

	vhd := ""
	for _, path := range artifact.Files() {
		if strings.HasSuffix(path, ".vhd") {
			vhd = path
			break
		}
	}

	if vhd == "" {
		return nil, false, fmt.Errorf("No VHD artifact file found")
	}

	vmType := artifact.State("virtualizationType")
	if vhd == "" {
		return nil, false, fmt.Errorf("Virtualization type wasn't specified")
	}

	requiresHVM := "false"
	if strings.ToLower(vmType.(string)) == "hvm" {
		requiresHVM = "true"
	}

	dir, file := path.Split(vhd)

	copyFromUrl := ""
	uploadFile := vhd

	if p.config.CompressVhd {
		uploadFile = dir + "template.vhd.gzip"
		os.Remove ( uploadFile ) // remove any existing file to ensure no corruption

		ui.Message(fmt.Sprintf("Compressing '%s' to '%s'", vhd, uploadFile))	

		err := compressVhd  ( vhd, uploadFile)
		if err != nil {
			return nil, false, fmt.Errorf("Error compressing template '%s': %s", vhd, err)
		}

		if strings.HasSuffix (p.config.DownloadUrl, "/" ) {
			copyFromUrl = p.config.DownloadUrl + "template.vhd.gzip"
		} else {
			copyFromUrl = fmt.Sprintf("%s/%s", p.config.DownloadUrl, "template.vhd.gzip") 
		}
	} else {
		if strings.HasSuffix (p.config.DownloadUrl, "/" ) {
			copyFromUrl = p.config.DownloadUrl + file
		} else {
			copyFromUrl = fmt.Sprintf("%s/%s", p.config.DownloadUrl, file) 
		}
	}

	ui.Message(fmt.Sprintf("Uploading %s to CloudStack", uploadFile))

	// Create a new caching client
	acs, err := gocs.NewCachingClient(p.config.ApiUrl, p.config.ApiKey, p.config.Secret, 0, false)
	if err != nil {
		return nil, false, fmt.Errorf("Connection error: %s", err)
	}

	ostypeid, err := acs.Request("listOsTypes", fmt.Sprintf("description:%s", p.config.OsType))
	if err != nil {
		return nil, false, fmt.Errorf("Error locating OS type '%s': %s", p.config.OsType, err)
	}

	ui.Say(fmt.Sprintf("OS '%s' has id '%s'", p.config.OsType, ostypeid))	

	zoneid, err := acs.Request("listZones", fmt.Sprintf("name:%s", p.config.Zone))
	if err != nil {
		return nil, false, fmt.Errorf("Error locating Zone '%s': %s", p.config.Zone, err)
	}

	ui.Say(fmt.Sprintf("Zone '%s' has id '%s'", p.config.Zone, zoneid))	

	templateid, err := acs.Request("registerTemplate", fmt.Sprintf("displaytext:%s, ostypeid:%s, format:vhd, hypervisor:xenserver, name:%s, zoneid:%s, url:%s, requireshvm:%t, passwordenabled:%t, sshkeyenabled:%t, isdynamicallyscalable:%t",
									p.config.DisplayText, ostypeid, p.config.TemplateName, zoneid, copyFromUrl, requiresHVM, p.config.PwdEnabled, p.config.SshEnabled, p.config.HasTools))
	if err != nil {
		return nil, false, fmt.Errorf("Error registering template '%s': %s", p.config.TemplateName, err)
	}

	ui.Say(fmt.Sprintf("Template registered as '%s'", templateid))	


	lastStatus := ""
	downloadStarted := false

	iterations := int(p.config.UploadTimer) / 5

	for i := 0; i < iterations; i++ {
		templateDetails, err := acs.RawRequest("listTemplates", fmt.Sprintf("id:%s, templatefilter:all", templateid))
		if err != nil {
			return nil, false, fmt.Errorf("Error locating template '%s': %s", templateid, err)
		}

		jsonResponse, err := gocs.UnmarshalResponse ("template", templateDetails)
		if err != nil {
			return nil, false, fmt.Errorf("Error unmarshalling template repsonse: %s", err)
		}

		// a normal download will see the "status" start blank, then become non-blank in a few seconds, ending with isready:true
		// anything else is an error

		var status []TemplateResponse 
		if err := json.Unmarshal(jsonResponse, &status ); err != nil {
			return nil, false, fmt.Errorf("Error unmarshalling template repsonse: %s", err)
		}

		if status[0].Status != lastStatus {
			ui.Say(fmt.Sprintf("Template processing status '%s'", status[0].Status))	
			lastStatus = status[0].Status
			if strings.TrimSpace(lastStatus) != "" && !downloadStarted {
				if strings.Contains (lastStatus, "%") {
					downloadStarted = true
				} else if strings.HasPrefix (lastStatus, "Failed") {
					return nil, false, fmt.Errorf("Template '%s' processing aborted due to error: '%s'", p.config.TemplateName, strings.TrimSpace(lastStatus))
				} else {
					return nil, false, fmt.Errorf("Template '%s' download aborted due to error: '%s'", p.config.TemplateName, strings.TrimSpace(lastStatus))
				}
			}
		}

		if status[0].IsReady {
			ui.Say(fmt.Sprintf("Template '%s' is ready ", p.config.TemplateName))	
			break
		}

		time.Sleep(time.Duration(5)*time.Second)
	}


	return artifact, false, nil
}


func compressVhd ( templateVhd string, gzFilename string ) (err error) {

	gzFile, err := os.Create(gzFilename)
	if err != nil {
		return fmt.Errorf( "Failed creating file for compressed archive %s: %s", gzFilename, err)
	}
	defer gzFile.Close()

	gzipWriter := gzip.NewWriter(gzFile)
	defer gzipWriter.Close()

	// Open the target file for archiving and compressing.
	fileReader, err := os.Open(templateVhd)
	if err != nil {
		return fmt.Errorf("Failed opening file '%s' to write compressed archive. %s", templateVhd, err)
	}
	defer fileReader.Close()

	if _, err = io.Copy(gzipWriter, fileReader); err != nil {
		return fmt.Errorf("Failed copying file %s to archive: %s", templateVhd, err)
	}

	return nil
}