//go:build kyse

package tags

import (
	"github.com/arandu-io/kyse/components"

	tags "github.com/hyz-is/arandu-tags"
)

@go
// PickerData is what an application hands this fragment.
//
// It is the one screen here that no route of this module answers, and that is
// the same decision the package makes everywhere else about associations:
// whether somebody may tag an article is a question about the article, and the
// policy that answers it belongs to whoever owns that table. So the application
// draws this inside a screen of its own, from a handler that has already asked,
// and Action is its route rather than one of ours.
type PickerData = tags.PickerData
@endgo

<section class="grid gap-4">
	<h2 class="text-sm font-semibold tracking-tight">{{ .Labels.T("screen.picker_title") }}</h2>

	<form class="grid gap-3" hx-post="{{ .Action }}">
		<input type="hidden" name="_token" value="{{ .Token }}">
		<input type="hidden" name="type" value="{{ .Taxonomy }}">

		{{-- Every label of the taxonomy is a checkbox, and the ones the entity
		     carries are the ones ticked. The form posts the whole set rather
		     than what changed, which is what makes it answerable with one sync:
		     a form that posted only the difference would describe a state it had
		     read some time ago, and two people editing would each undo the
		     other. --}}
		@if(len(.Available) > 0)
			<ul class="grid gap-2">
				@foreach(.Available as row)
					<li>
						{!! components.Checkbox(components.CheckboxProps{
							Name:    "id",
							ID:      "tag-" + row.ID,
							Value:   row.ID,
							Label:   row.Label,
							Checked: row.Held,
						}) !!}
					</li>
				@endforeach
			</ul>
			<div>
				{!! components.Button(components.ButtonProps{
					Label:   .Labels.T("control.save"),
					Type:    "submit",
					Size:    "sm",
					Variant: "outline",
				}) !!}
			</div>
		@else
			<p class="text-muted-foreground text-sm">{{ .Labels.T("screen.picker_empty") }}</p>
		@endif
	</form>

	@if(len(.Carried) > 0)
		<ul class="flex flex-wrap gap-2">
			@foreach(.Carried as row)
				<li>{!! components.Badge(components.BadgeProps{Label: row.Label, Variant: "secondary"}) !!}</li>
			@endforeach
		</ul>
	@endif
</section>
