package openstack

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/attributestags"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/external"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/provider"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
)

func resourceNetworkingNetworkV2() *schema.Resource {
	return &schema.Resource{
		Create: resourceNetworkingNetworkV2Create,
		Read:   resourceNetworkingNetworkV2Read,
		Update: resourceNetworkingNetworkV2Update,
		Delete: resourceNetworkingNetworkV2Delete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(10 * time.Minute),
			Delete: schema.DefaultTimeout(10 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"region": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Computed: true,
			},
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},
			"description": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},
			"admin_state_up": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
				Computed: true,
			},
			"shared": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
				Computed: true,
			},
			"external": &schema.Schema{
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: false,
				Computed: true,
			},
			"tenant_id": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Computed: true,
			},
			"segments": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"physical_network": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
						},
						"network_type": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
						},
						"segmentation_id": &schema.Schema{
							Type:     schema.TypeInt,
							Optional: true,
							ForceNew: true,
						},
					},
				},
			},
			"value_specs": &schema.Schema{
				Type:     schema.TypeMap,
				Optional: true,
				ForceNew: true,
			},
			"tags": &schema.Schema{
				Type:     schema.TypeSet,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"availability_zone_hints": &schema.Schema{
				Type:     schema.TypeList,
				Computed: true,
				ForceNew: true,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
		},
	}
}

func resourceNetworkingNetworkV2Create(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	networkingClient, err := config.networkingV2Client(GetRegion(d, config))
	if err != nil {
		return fmt.Errorf("Error creating OpenStack networking client: %s", err)
	}

	createOpts := NetworkCreateOpts{
		networks.CreateOpts{
			Name:                  d.Get("name").(string),
			Description:           d.Get("description").(string),
			TenantID:              d.Get("tenant_id").(string),
			AvailabilityZoneHints: resourceNetworkingAvailabilityZoneHintsV2(d),
		},
		MapValueSpecs(d),
	}

	asuRaw := d.Get("admin_state_up").(string)
	if asuRaw != "" {
		asu, err := strconv.ParseBool(asuRaw)
		if err != nil {
			return fmt.Errorf("admin_state_up, if provided, must be either 'true' or 'false'")
		}
		createOpts.AdminStateUp = &asu
	}

	sharedRaw := d.Get("shared").(string)
	if sharedRaw != "" {
		shared, err := strconv.ParseBool(sharedRaw)
		if err != nil {
			return fmt.Errorf("shared, if provided, must be either 'true' or 'false': %v", err)
		}
		createOpts.Shared = &shared
	}

	segments := resourceNetworkingNetworkV2Segments(d)

	isExternal := d.Get("external").(bool)
	n := &networks.Network{}
	if len(segments) > 0 {
		providerCreateOpts := provider.CreateOptsExt{
			CreateOptsBuilder: createOpts,
			Segments:          segments,
		}
		if isExternal {
			createExternalOpts := external.CreateOptsExt{
				CreateOptsBuilder: providerCreateOpts,
				External:          &isExternal,
			}
			log.Printf("[DEBUG] Create Options: %#v", createExternalOpts)
			n, err = networks.Create(networkingClient, createExternalOpts).Extract()
		} else {
			log.Printf("[DEBUG] Create Options: %#v", providerCreateOpts)
			n, err = networks.Create(networkingClient, providerCreateOpts).Extract()
		}
	} else {
		if isExternal {
			createExternalOpts := external.CreateOptsExt{
				CreateOptsBuilder: createOpts,
				External:          &isExternal,
			}
			log.Printf("[DEBUG] Create Options: %#v", createExternalOpts)
			n, err = networks.Create(networkingClient, createExternalOpts).Extract()
		} else {
			log.Printf("[DEBUG] Create Options: %#v", createOpts)
			n, err = networks.Create(networkingClient, createOpts).Extract()
		}
	}

	if err != nil {
		return fmt.Errorf("Error creating OpenStack Neutron network: %s", err)
	}

	log.Printf("[INFO] Network ID: %s", n.ID)

	tags := networkV2AttributesTags(d)
	if len(tags) > 0 {
		tagOpts := attributestags.ReplaceAllOpts{Tags: tags}
		tags, err := attributestags.ReplaceAll(networkingClient, "networks", n.ID, tagOpts).Extract()
		if err != nil {
			return fmt.Errorf("Error creating Tags on Network: %s", err)
		}
		log.Printf("[DEBUG] Set Tags = %+v on Network %+v", tags, n.ID)
	}

	log.Printf("[DEBUG] Waiting for Network (%s) to become available", n.ID)

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"BUILD"},
		Target:     []string{"ACTIVE"},
		Refresh:    waitForNetworkActive(networkingClient, n.ID),
		Timeout:    d.Timeout(schema.TimeoutCreate),
		Delay:      5 * time.Second,
		MinTimeout: 3 * time.Second,
	}

	_, err = stateConf.WaitForState()

	d.SetId(n.ID)

	return resourceNetworkingNetworkV2Read(d, meta)
}

func resourceNetworkingNetworkV2Read(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	networkingClient, err := config.networkingV2Client(GetRegion(d, config))
	if err != nil {
		return fmt.Errorf("Error creating OpenStack networking client: %s", err)
	}

	var n struct {
		networks.Network
		external.NetworkExternalExt
	}
	err = networks.Get(networkingClient, d.Id()).ExtractInto(&n)
	if err != nil {
		return CheckDeleted(d, err, "network")
	}

	log.Printf("[DEBUG] Retrieved Network %s: %+v", d.Id(), n)

	d.Set("name", n.Name)
	d.Set("description", n.Description)
	d.Set("admin_state_up", strconv.FormatBool(n.AdminStateUp))
	d.Set("shared", strconv.FormatBool(n.Shared))
	d.Set("external", strconv.FormatBool(n.External))
	d.Set("tenant_id", n.TenantID)
	d.Set("region", GetRegion(d, config))
	d.Set("tags", n.Tags)

	if err := d.Set("availability_zone_hints", n.AvailabilityZoneHints); err != nil {
		log.Printf("[DEBUG] unable to set availability_zone_hints: %s", err)
	}

	return nil
}

func resourceNetworkingNetworkV2Update(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	networkingClient, err := config.networkingV2Client(GetRegion(d, config))
	if err != nil {
		return fmt.Errorf("Error creating OpenStack networking client: %s", err)
	}

	var updateOpts networks.UpdateOpts
	if d.HasChange("name") {
		updateOpts.Name = d.Get("name").(string)
	}
	if d.HasChange("description") {
		description := d.Get("description").(string)
		updateOpts.Description = &description
	}
	if d.HasChange("tags") {
		tags := networkV2AttributesTags(d)
		tagOpts := attributestags.ReplaceAllOpts{Tags: tags}
		tags, err := attributestags.ReplaceAll(networkingClient, "networks", d.Id(), tagOpts).Extract()
		if err != nil {
			return fmt.Errorf("Error updating Tags on Network: %s", err)
		}
		log.Printf("[DEBUG] Updated Tags = %+v on Network %+v", tags, d.Id())
	}
	if d.HasChange("admin_state_up") {
		asuRaw := d.Get("admin_state_up").(string)
		if asuRaw != "" {
			asu, err := strconv.ParseBool(asuRaw)
			if err != nil {
				return fmt.Errorf("admin_state_up, if provided, must be either 'true' or 'false'")
			}
			updateOpts.AdminStateUp = &asu
		}
	}
	if d.HasChange("shared") {
		sharedRaw := d.Get("shared").(string)
		if sharedRaw != "" {
			shared, err := strconv.ParseBool(sharedRaw)
			if err != nil {
				return fmt.Errorf("shared, if provided, must be either 'true' or 'false': %v", err)
			}
			updateOpts.Shared = &shared
		}
	}
	isExternal := false
	if d.HasChange("external") {
		isExternal = d.Get("external").(bool)
	}

	if isExternal {
		externalUpdateOpts := external.UpdateOptsExt{
			UpdateOptsBuilder: updateOpts,
			External:          &isExternal,
		}
		log.Printf("[DEBUG] Updating Network %s with options: %+v", d.Id(), externalUpdateOpts)
		_, err = networks.Update(networkingClient, d.Id(), externalUpdateOpts).Extract()
	} else {
		log.Printf("[DEBUG] Updating Network %s with options: %+v", d.Id(), updateOpts)
		_, err = networks.Update(networkingClient, d.Id(), updateOpts).Extract()
	}

	if err != nil {
		return fmt.Errorf("Error updating OpenStack Neutron Network: %s", err)
	}

	return resourceNetworkingNetworkV2Read(d, meta)
}

func resourceNetworkingNetworkV2Delete(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	networkingClient, err := config.networkingV2Client(GetRegion(d, config))
	if err != nil {
		return fmt.Errorf("Error creating OpenStack networking client: %s", err)
	}

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"ACTIVE"},
		Target:     []string{"DELETED"},
		Refresh:    waitForNetworkDelete(networkingClient, d.Id()),
		Timeout:    d.Timeout(schema.TimeoutDelete),
		Delay:      5 * time.Second,
		MinTimeout: 3 * time.Second,
	}

	_, err = stateConf.WaitForState()
	if err != nil {
		return fmt.Errorf("Error deleting OpenStack Neutron Network: %s", err)
	}

	d.SetId("")
	return nil
}

func resourceNetworkingNetworkV2Segments(d *schema.ResourceData) (providerSegments []provider.Segment) {
	segments := d.Get("segments").([]interface{})
	for _, v := range segments {
		var segment provider.Segment
		segmentMap := v.(map[string]interface{})

		if v, ok := segmentMap["physical_network"].(string); ok {
			segment.PhysicalNetwork = v
		}

		if v, ok := segmentMap["network_type"].(string); ok {
			segment.NetworkType = v
		}

		if v, ok := segmentMap["segmentation_id"].(int); ok {
			segment.SegmentationID = v
		}

		providerSegments = append(providerSegments, segment)
	}
	return
}

func waitForNetworkActive(networkingClient *gophercloud.ServiceClient, networkId string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		n, err := networks.Get(networkingClient, networkId).Extract()
		if err != nil {
			return nil, "", err
		}

		log.Printf("[DEBUG] OpenStack Neutron Network: %+v", n)
		if n.Status == "DOWN" || n.Status == "ACTIVE" {
			return n, "ACTIVE", nil
		}

		return n, n.Status, nil
	}
}

func waitForNetworkDelete(networkingClient *gophercloud.ServiceClient, networkId string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		log.Printf("[DEBUG] Attempting to delete OpenStack Network %s.\n", networkId)

		n, err := networks.Get(networkingClient, networkId).Extract()
		if err != nil {
			if _, ok := err.(gophercloud.ErrDefault404); ok {
				log.Printf("[DEBUG] Successfully deleted OpenStack Network %s", networkId)
				return n, "DELETED", nil
			}
			return n, "ACTIVE", err
		}

		err = networks.Delete(networkingClient, networkId).ExtractErr()
		if err != nil {
			if _, ok := err.(gophercloud.ErrDefault404); ok {
				log.Printf("[DEBUG] Successfully deleted OpenStack Network %s", networkId)
				return n, "DELETED", nil
			}
			if errCode, ok := err.(gophercloud.ErrUnexpectedResponseCode); ok {
				if errCode.Actual == 409 {
					return n, "ACTIVE", nil
				}
			}
			return n, "ACTIVE", err
		}

		log.Printf("[DEBUG] OpenStack Network %s still active.\n", networkId)
		return n, "ACTIVE", nil
	}
}
