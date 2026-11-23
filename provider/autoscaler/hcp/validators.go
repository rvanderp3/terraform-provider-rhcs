// Copyright Red Hat
// SPDX-License-Identifier: Apache-2.0

package hcp

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func validateScaleDownConfig(config types.Object, enabled types.Bool) diag.Diagnostics {
	var diags diag.Diagnostics
	if config.IsNull() || config.IsUnknown() || enabled.IsNull() || enabled.IsUnknown() || enabled.ValueBool() {
		return diags
	}
	values := config.Attributes()
	for _, name := range []string{
		unneededTimeAttribute, utilizationThresholdAttribute,
		delayAfterAddAttribute, delayAfterDeleteAttribute, delayAfterFailureAttribute,
	} {
		if value := values[name]; !value.IsNull() && !value.IsUnknown() {
			diags.AddAttributeError(path.Root("scale_down").AtName(name), "Conflicting scale-down configuration",
				fmt.Sprintf("scale_down.%s cannot be configured when scale_down.enabled is false. "+
					"Remove this tuning setting to disable scale-down, or set enabled = true to enable it.", name))
		}
	}
	return diags
}
