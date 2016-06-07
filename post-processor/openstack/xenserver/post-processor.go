package xenserver

import (
	"fmt"
	"github.com/mitchellh/packer/common"
	"github.com/mitchellh/packer/helper/config"
	"github.com/mitchellh/packer/packer"
	"github.com/mitchellh/packer/template/interpolate"
	"strings"
	"strconv"
	"github.com/xenserverarmy/go-osglance/identity/v2"
	"github.com/xenserverarmy/go-osglance/image/v1"
	"time"
	"net/http"
	"os"
	"io"
	"path"
	"archive/tar"
	"compress/gzip"
)

var builtins = map[string]string{
	"packer.xenserver": "xenserver",
}

type Config struct {
	common.PackerConfig `mapstructure:",squash"`

	IdentityHost	string `mapstructure:"identity_url"`
	Username    	string `mapstructure:"username"`
	Password 	string `mapstructure:"password"`

	ProjectName	string `mapstructure:"project_name"`
	ImageRegion 	string `mapstructure:"image_region"`
	ImageName	string `mapstructure:"image_name"`
	DownloadUrl	string `mapstructure:"download_url"`
	MinDisk	int64 	`mapstructure:"min_disk"`
	MinRam		int64 `mapstructure:"min_ram"`
	IsPublic	bool `mapstructure:"is_public"`
	UploadTimer	uint `mapstructure:"upload_timer"`

	ctx interpolate.Context
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

	// Accumulate any errors
	errs := new(packer.MultiError)

	// First define all our templatable parameters that are _required_
	templates := map[string]*string{
		"identity_url":     &p.config.IdentityHost,
		"username":    	&p.config.Username,
		"password":     &p.config.Password,
		"image_region":   &p.config.ImageRegion,
		"image_name":  &p.config.ImageName,
		"download_url":   &p.config.DownloadUrl,
	}

	for key, ptr := range templates {
		if *ptr == "" {
			errs = packer.MultiErrorAppend(
				errs, fmt.Errorf("%s must be set", key))
		}
	}

	if p.config.UploadTimer == 0 {
		p.config.UploadTimer  = 1200
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
		if strings.HasSuffix(path, ".vhd") && !strings.HasSuffix(path, "/0.vhd") {
			// 0.vhd is special in OpenStack on XenServer so should never be present
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

	vmMode := ""
	if strings.ToLower(vmType.(string)) == "hvm" {
		vmMode = "hvm"
	} else {
		vmMode = "xen"
	}

	ui.Message(fmt.Sprintf("Uploading %s with mode %s to OpenStack Glance", vhd, vmMode))

	// first we need to rename the artifact to 0.vhd, then compress it into a tgz
	
	dir, _ := path.Split(vhd)

	osvhd := dir + "0.vhd"
	os.Remove ( osvhd )	// remove any existing file to ensure no corruption

	err := os.Rename ( vhd, osvhd )
	if err != nil {
		return nil, false, fmt.Errorf("Error renaming '%s' to '%s': %s", vhd, osvhd, err)		
	}

	ostgz := dir + "0.vhd.tgz"
	os.Remove ( ostgz ) // remove any existing file to ensure no corruption

	ui.Message(fmt.Sprintf("Compressing '%s' to '%s'", vhd, ostgz))	

	err = compressVhd  ( osvhd, ostgz )
	
	ui.Say("Authenticating to identity service")
	// Authenticate with a username, password, tenant id.
	auth, err := identity.AuthUserNameTenantName(p.config.IdentityHost,
		p.config.Username,
		p.config.Password,
		p.config.ProjectName)

	if err != nil {
		return nil, false, fmt.Errorf("There was an error authenticating:", err)
	}

	if !auth.Access.Token.Expires.After(time.Now()) {
		return nil, false, fmt.Errorf("There was an error. The auth token has an invalid expiration.")
	}

	// Find the endpoint for the image service.
	url := ""
	for _, svc := range auth.Access.ServiceCatalog {
		if svc.Type == "image" {
			for _, ep := range svc.Endpoints {
				if ep.Region == p.config.ImageRegion {
					url = ep.PublicURL
					break
				}
			}
		}
		
	}

	// the url returned will like have multiple versions for the service; get v1
	if url == "" {
		return nil, false, fmt.Errorf("v1 image service url not found during authentication")
	} else {
		ui.Message(fmt.Sprintf ("Endpoint image url found: '%s'", url))
	}

	imageService := image.Service{TokenID: auth.Access.Token.Id, Client: *http.DefaultClient, URL: url}

	validGlanceService, updatedUrl, err := imageService.GetV1Interface ()
	if err != nil {
		return nil, false, fmt.Errorf("Error validating Glance service: '%s'", err)
	} else if !validGlanceService {
		return nil, false, fmt.Errorf("Found invalid Glance service")
	} else {
		imageService.URL = updatedUrl
		ui.Message(fmt.Sprintf ("Versioned endpoint Glance url found: '%s'", imageService.URL))
	}

	copyFromUrl := ""
	if strings.HasSuffix (p.config.DownloadUrl, "/" ) {
		copyFromUrl = p.config.DownloadUrl + "0.vhd.tgz"
	} else {
		copyFromUrl = fmt.Sprintf("%s/%s", p.config.DownloadUrl, "0.vhd.tgz") 
	}

	var configuredRam int64 = p.config.MinRam
	value, found := artifact.State("ramSize").(string) // value will be in MB 
	if found {
		result, _ := strconv.ParseInt ( value, 10, 64 )
		if result > configuredRam {
			configuredRam = result
		} 
	} 

	var configuredDisk int64 = p.config.MinDisk
	value, found = artifact.State("diskSize").(string) // value will be in MB 
	if found {
		result, _ := strconv.ParseInt ( value, 10, 64 )
		if result > configuredDisk {
			configuredDisk = result
			ui.Message(fmt.Sprintf ("Overriding disk size config: %d actual: %d", p.config.MinDisk, configuredDisk ))
		} 
	} 

	ui.Message(fmt.Sprintf ("Requesting Glance to copy image: '%s'", copyFromUrl ))

	uploadObject := image.UploadParameters {Name: p.config.ImageName, 
		DiskFormat: "vhd", 
		ContainerFormat: "ovf", 
		CopyFromUrl: copyFromUrl,
		IsPublic: true, 
		MinDisk: configuredDisk, 
		MinRam: configuredRam}

	imageId, imageStatus, err := imageService.ReserveImage (uploadObject, "xen", vmMode)
	if err != nil {
		return nil, false, fmt.Errorf("Unable to reserve image: '%s'", err)
	}

	ui.Say (fmt.Sprintf ("Reserved image has id '%s' and status '%s'", imageId, imageStatus))

	lastStatus := imageStatus
	downloadStarted := false

	for i := 0; i <100; i++ {

		status, err := imageService.ImageStatus (imageId)
		if err != nil {
			return nil, false, fmt.Errorf("Error obtaining status for iamge '%s': %s", imageId, err)
		}

		// a normal image create will see the status start as "queued", then become "saving" in a few seconds, ending with "active"
		// anything else is an error

		if status != lastStatus {
			ui.Message (fmt.Sprintf("Image processing status '%s'", status))
			lastStatus = status
			if strings.ToLower(lastStatus) == "saving" && !downloadStarted {
				downloadStarted = true
			} else if status == "active" {
				ui.Say (fmt.Sprintf("Image '%s' is ready ", uploadObject.Name))
				break
			} else {
				return nil, false, fmt.Errorf("Image '%s' download aborted due to error: '%s'", p.config.ImageName, lastStatus)
			}
		}

		time.Sleep(time.Duration(5)*time.Second)
	}

	return artifact, false, nil
}


func compressVhd ( osvhd string, ostgz string ) (err error) {

	tgzWriter, err := os.Create(ostgz)
	if err != nil {
		return fmt.Errorf( "Failed creating file for compressed archive %s: %s", ostgz, err)
	}
	defer tgzWriter.Close()

	gzipWriter := gzip.NewWriter(tgzWriter)
	defer gzipWriter.Close()

	fi, err := os.Stat(osvhd)
	if err != nil {
		return fmt.Errorf("Failed obtaining fileinfo for %s: %s", osvhd, err)
	}

	target, _ := os.Readlink(osvhd)
	header, err := tar.FileInfoHeader(fi, target)
	if err != nil {
		return fmt.Errorf("Failed creating archive header for %s: %s", osvhd, err)
	}

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	if err := tarWriter.WriteHeader(header); err != nil {
		return fmt.Errorf("Failed writing archive header for %s: %s", osvhd, err)
	}

	// Open the target file for archiving and compressing.
	fileReader, err := os.Open(osvhd)
	if err != nil {
		return fmt.Errorf("Failed opening file '%s' to write compressed archive. %s", osvhd, err)
	}
	defer fileReader.Close()

	if _, err = io.Copy(tarWriter, fileReader); err != nil {
		return fmt.Errorf("Failed copying file %s to archive: %s", osvhd, err)
	}

	return nil
}