package openstack

import (
	"fmt"
	"github.com/gophercloud/gophercloud/openstack/clustering/v1/profiles"
	"log"
	"time"

	"github.com/hashicorp/terraform/helper/schema"
)

func resourceClusteringProfileV1() *schema.Resource {
	return &schema.Resource{
		Create: resourceClusteringProfileV1Create,
		Read:   resourceClusteringProfileV1Read,
		Update: resourceClusteringProfileV1Update,
		Delete: resourceClusteringProfileV1Delete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(10 * time.Minute),
			Delete: schema.DefaultTimeout(10 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"metadata": &schema.Schema{
				Type:     schema.TypeMap,
				Required: false,
				ForceNew: true,
			},
			"spec": &schema.Schema{
				Type:     schema.TypeMap,
				Required: true,
				ForceNew: true,
			},
		},
	}
}

func resourceClusteringProfileV1Create(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	clusteringClient, err := config.clusteringV1Client(GetRegion(d, config))
	if err != nil {
		return fmt.Errorf("Error creating OpenStack clustering client: %s", err)
	}

	createOpts := profiles.CreateOpts{
		Name: d.Get("name").(string),
		Spec: profiles.Spec{
			Type:       "os.nova.server",
			Version:    "1.0",
			Properties: d.Get("spec").(map[string]interface{}),
		},
	}

	log.Printf("[DEBUG] openstack_clustering_profile_v1 create options: %#v", createOpts)

	s, err := profiles.Create(clusteringClient, createOpts).Extract()
	if err != nil {
		return fmt.Errorf("Error creating openstack_clustering_profile_v1: %s", err)
	}

	d.SetId(s.ID)

	log.Printf("[DEBUG] Created openstack_clustering_profile_v1 %s: %#v", s.ID, s)
	return resourceClusteringProfileV1Read(d, meta)
}

func resourceClusteringProfileV1Read(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	clusteringClient, err := config.clusteringV1Client(GetRegion(d, config))
	if err != nil {
		return fmt.Errorf("Error creating OpenStack clustering client: %s", err)
	}

	s, err := profiles.Get(clusteringClient, d.Id()).Extract()
	if err != nil {
		return CheckDeleted(d, err, "Error retrieving openstack_clustering_profile_v1")
	}

	log.Printf("[DEBUG] Retrieved openstack_containerinfra_clustertemplate_v1 %s: %#v", d.Id(), s)

	d.Set("created_at", s.CreatedAt)
	d.Set("domain", s.Domain)
	d.Set("id", s.ID)
	d.Set("metadata", s.Metadata)
	d.Set("project", s.Project)
	d.Set("spec", s.Spec.Properties)
	d.Set("type", s.Type)
	d.Set("updated_at", s.UpdatedAt)
	d.Set("user", s.User)
	d.Set("region", GetRegion(d, config))
	d.Set("name", s.Name)

	if err := d.Set("created_at", s.CreatedAt.Format(time.RFC3339)); err != nil {
		log.Printf("[DEBUG] Unable to set openstack_clustering_profile_v1 created_at: %s", err)
	}
	if err := d.Set("updated_at", s.UpdatedAt.Format(time.RFC3339)); err != nil {
		log.Printf("[DEBUG] Unable to set openstack_clustering_profile_v1 updated_at: %s", err)
	}

	return nil
}

func resourceClusteringProfileV1Update(d *schema.ResourceData, meta interface{}) error {
	//config := meta.(*Config)
	//clusteringClient, err := config.clusteringV1Client(GetRegion(d, config))
	//if err != nil {
	//	return fmt.Errorf("Error creating OpenStack clustering client: %s", err)
	//}

	//updateOpts := []profiles.UpdateOptsBuilder{}
	//
	//if d.HasChange("name") {
	//	v := d.Get("name").(string)
	//	updateOpts = append(updateOpts, profiles.UpdateOpts{
	//		Name: v,
	//		Metadata: d.Get("metadata").(map[string]interface{}),
	//	})
	//}
	//
	//log.Printf(
	//	"[DEBUG] Updating openstack_clustering_profile_v1 %s with options: %#v", d.Id(), updateOpts)
	//
	//_, err = profiles.Update(clusteringClient, d.Id(), updateOpts).Extract()
	//if err != nil {
	//	return fmt.Errorf("Error updating openstack_clustering_profile_v1 %s: %s", d.Id(), err)
	//}

	return resourceClusteringProfileV1Read(d, meta)
}

func resourceClusteringProfileV1Delete(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	clusteringClient, err := config.clusteringV1Client(GetRegion(d, config))
	if err != nil {
		return fmt.Errorf("Error creating OpenStack clustering client: %s", err)
	}

	if err := profiles.Delete(clusteringClient, d.Id()).ExtractErr(); err != nil {
		return CheckDeleted(d, err, "Error deleting openstack_clustering_profile_v1")
	}

	return nil
}
