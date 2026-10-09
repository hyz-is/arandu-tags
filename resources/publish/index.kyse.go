//go:build kyse

package tags

import (
	"github.com/arandu-io/kyse/components"

	tags "github.com/hyz-is/arandu-tags"
)

@go
// IndexData is what the handler hands this page.
//
// It is an alias rather than a struct declared here, so the shape is written
// once, beside the code that fills it. A second declaration is two shapes kept
// in step by hand, and a field missing from one of them is a blank space on a
// page that answered 200.
type IndexData = tags.IndexPageData
@endgo

@extends('layouts.app')

@section('content')
	<div class="flex flex-wrap items-start justify-between gap-4">
		<div>
			<h1 class="text-2xl font-semibold tracking-tight">{{ .Labels.T("screen.index_title") }}</h1>
			<p class="text-muted-foreground mt-1 text-sm">{{ .Labels.T("screen.index_lead") }}</p>
		</div>
		<a class="btn" data-variant="outline" data-size="sm" href="{{ .OrderURL }}">
			{{ .Labels.T("control.order") }}
		</a>
	</div>

	{{-- The taxonomies are links and not a select, because each one is a page
	     with an address: a narrowed listing somebody can send to a colleague is
	     worth more than one that only exists in their tab. --}}
	@if(len(.TaxonomyLinks) > 1)
		<nav class="mt-6 flex flex-wrap items-center gap-2">
			@foreach(.TaxonomyLinks as taxonomy)
				<a class="btn" data-size="sm" data-variant="ghost" href="{{ taxonomy.URL }}">
					{{ taxonomy.Label }}
				</a>
			@endforeach
		</nav>
	@endif

	<form class="mt-6 flex items-end gap-2" method="get" action="{{ .SearchURL }}">
		<input type="hidden" name="type" value="{{ .Taxonomy }}">
		<div class="grow">
			{!! components.Label(components.LabelProps{For: "q", Text: .Labels.T("field.search")}) !!}
			{!! components.Input(components.InputProps{
				Name:        "q",
				ID:          "q",
				Type:        "search",
				Value:       .Search,
				Placeholder: .Labels.T("control.search_placeholder"),
			}) !!}
		</div>
		{!! components.Button(components.ButtonProps{Label: .Labels.T("field.search"), Type: "submit", Variant: "outline"}) !!}
	</form>

	@if(len(.Rows) > 0)
		<ul class="mt-6 grid gap-3">
			@foreach(.Rows as row)
				<li class="card flex flex-wrap items-center justify-between gap-3 p-4">
					<div class="min-w-0">
						<a class="text-sm font-semibold hover:underline" href="{{ row.URL }}">{{ row.Label }}</a>
						<p class="text-muted-foreground mt-1 truncate text-xs">{{ row.Slug }}</p>
					</div>
					<div class="flex items-center gap-2">
						{!! components.Badge(components.BadgeProps{Label: row.TaxonomyLabel, Variant: "outline"}) !!}
						<span class="text-muted-foreground text-xs">{{ row.Position }}</span>
					</div>
				</li>
			@endforeach
		</ul>
	@else
		<p class="text-muted-foreground mt-8 text-sm">{{ .Labels.T("screen.index_empty") }}</p>
	@endif

	@if(.NextURL != "")
		<div class="mt-6">
			<a class="btn" data-variant="outline" data-size="sm"
			   href="{{ .NextURL }}">
				{{ .Labels.T("control.move_down") }}
			</a>
		</div>
	@endif

	<form class="mt-10 grid gap-3 border-t pt-6" method="post" action="{{ .StoreURL }}">
		@csrf
		<input type="hidden" name="type" value="{{ .Taxonomy }}">
		{!! components.Field(components.FieldProps{
			Name:        "name",
			Label:       .Labels.T("field.name"),
			Placeholder: .Labels.T("control.name_placeholder"),
			Required:    true,
			Page:        .Form(),
		}) !!}
		<div>
			{!! components.Button(components.ButtonProps{Label: .Labels.T("control.create"), Type: "submit"}) !!}
		</div>
	</form>
@endsection
