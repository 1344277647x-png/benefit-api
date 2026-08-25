package controller

import (
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func filterPricingByUsableGroups(pricing []model.Pricing, usableGroup map[string]string) []model.Pricing {
	if len(pricing) == 0 {
		return pricing
	}
	if len(usableGroup) == 0 {
		return []model.Pricing{}
	}

	filtered := make([]model.Pricing, 0, len(pricing))
	for _, item := range pricing {
		if common.StringsContains(item.EnableGroup, "all") {
			filtered = append(filtered, item)
			continue
		}
		for _, group := range item.EnableGroup {
			if _, ok := usableGroup[group]; ok {
				filtered = append(filtered, item)
				break
			}
		}
	}
	return filtered
}

func GetPricing(c *gin.Context) {
	pricing := model.GetPricing()
	userId, exists := c.Get("id")
	usableGroup := map[string]string{}
	groupRatio := map[string]float64{}
	for s, f := range ratio_setting.GetGroupRatioCopy() {
		groupRatio[s] = f
	}
	var group string
	if exists {
		user, err := model.GetUserCache(userId.(int))
		if err == nil {
			group = user.Group
			for g := range groupRatio {
				ratio, ok := ratio_setting.GetGroupGroupRatio(group, g)
				if ok {
					groupRatio[g] = ratio
				}
			}
		}
	}

	usableGroup = service.GetUserUsableGroups(group)
	pricing = filterPricingByUsableGroups(pricing, usableGroup)
	// check groupRatio contains usableGroup
	for group := range ratio_setting.GetGroupRatioCopy() {
		if _, ok := usableGroup[group]; !ok {
			delete(groupRatio, group)
		}
	}

	c.JSON(200, gin.H{
		"success":            true,
		"data":               pricing,
		"vendors":            model.GetVendors(),
		"group_ratio":        groupRatio,
		"usable_group":       usableGroup,
		"supported_endpoint": model.GetSupportedEndpointMap(),
		"auto_groups":        service.GetUserAutoGroup(group),
		"pricing_version":    "a42d372ccf0b5dd13ecf71203521f9d2",
	})
}

// GetUserIntegrationModels returns the endpoint capabilities that the current
// user can actually route. The public pricing catalog is intentionally broader
// in some deployments, so the skill center uses this authenticated view when
// the public catalog is unavailable or requires authentication.
func GetUserIntegrationModels(c *gin.Context) {
	user, err := model.GetUserCache(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	usableGroups := service.GetUserUsableGroups(user.Group)
	endpointGroups, err := model.GetEnabledModelEndpointGroups()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	vendors := make(map[int]string)
	for _, vendor := range model.GetVendors() {
		vendors[vendor.ID] = vendor.Name
	}

	type integrationModel struct {
		ModelName              string                  `json:"model_name"`
		VendorName             string                  `json:"vendor_name,omitempty"`
		SupportedEndpointTypes []constant.EndpointType `json:"supported_endpoint_types"`
	}
	data := make([]integrationModel, 0)
	for _, pricing := range model.GetPricing() {
		groupsByEndpoint := endpointGroups[pricing.ModelName]
		endpoints := make([]constant.EndpointType, 0, len(groupsByEndpoint))
		for endpointType, groups := range groupsByEndpoint {
			if hasUsableIntegrationGroup(groups, pricing.EnableGroup, usableGroups) {
				endpoints = append(endpoints, endpointType)
			}
		}
		if len(endpoints) == 0 {
			continue
		}
		sort.Slice(endpoints, func(i, j int) bool { return endpoints[i] < endpoints[j] })
		data = append(data, integrationModel{
			ModelName:              pricing.ModelName,
			VendorName:             vendors[pricing.VendorID],
			SupportedEndpointTypes: endpoints,
		})
	}

	c.JSON(200, gin.H{
		"success": true,
		"data":    data,
	})
}

func hasUsableIntegrationGroup(
	endpointGroups []string,
	pricingGroups []string,
	usableGroups map[string]string,
) bool {
	pricingAllowsAll := common.StringsContains(pricingGroups, "all")
	pricingGroupSet := make(map[string]struct{}, len(pricingGroups))
	for _, group := range pricingGroups {
		pricingGroupSet[group] = struct{}{}
	}
	for _, group := range endpointGroups {
		if _, ok := usableGroups[group]; !ok || !ratio_setting.ContainsGroupRatio(group) {
			continue
		}
		if pricingAllowsAll {
			return true
		}
		if _, ok := pricingGroupSet[group]; ok {
			return true
		}
	}
	return false
}

func ResetModelRatio(c *gin.Context) {
	defaultStr := ratio_setting.DefaultModelRatio2JSONString()
	err := model.UpdateOption("ModelRatio", defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	err = ratio_setting.UpdateModelRatioByJSONString(defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "重置模型倍率成功",
	})
}
