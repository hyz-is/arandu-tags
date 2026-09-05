//go:build kyse

package tags

import (
	"github.com/arandu-io/kyse/components"

	tags "github.com/hyz-is/arandu-tags"
)

@go
// EditData is what the handler hands this page.
type EditData = tags.EditPageData
@endgo

@extends('layouts.app')

@section('content')
	<div>
		<a class="text-muted-foreground text-sm hover:underline"
		   href="{{ .Prefix }}?type={{ .Row.Taxonomy }}">{{ .Labels.T("control.back") }}</a>
		<h1 class="mt-2 text-2xl font-semibold tracking-tight">{{ .Labels.T("screen.edit_title") }}</h1>
		<p class="text-muted-foreground mt-1 text-sm">{{ .Labels.T("screen.edit_lead") }}</p>
	</div>

	<dl class="mt-8 grid gap-2 text-sm">
		<div class="flex gap-2">
			<dt class="text-muted-foreground w-32">{{ .Labels.T("field.slug") }}</dt>
			<dd class="font-mono">{{ .Row.Slug }}</dd>
		</div>
		<div class="flex gap-2">
			<dt class="text-muted-foreground w-32">{{ .Labels.T("field.taxonomy") }}</dt>
			<dd>{{ .Row.TaxonomyLabel }}</dd>
		</div>
		<div class="flex gap-2">
			<dt class="text-muted-foreground w-32">{{ .Labels.T("field.position") }}</dt>
			<dd class="font-mono">{{ .Row.Position }}</dd>
		</div>
	</dl>

	{{-- The rename is a PATCH and the delete a DELETE, carried by htmx rather
	     than by the form element: a browser form sends GET or POST and nothing
	     else, and spelling the verb into a hidden field is a second place for a
	     route to be written down. --}}
	<form class="mt-8 grid gap-3" hx-patch="{{ .Prefix }}/{{ .Row.ID }}">
		@csrf
		{!! components.Field(components.FieldProps{
			Name:      "name",
			Label:     .Labels.T("field.name"),
			Value:     .Row.Name,
			Required:  true,
			Autofocus: true,
			Page:      .Form(),
		}) !!}
		<div class="flex items-center gap-2">
			{!! components.Button(components.ButtonProps{Label: .Labels.T("control.save"), Type: "submit"}) !!}
			<a class="btn" data-variant="ghost" href="{{ .Prefix }}?type={{ .Row.Taxonomy }}">
				{{ .Labels.T("control.cancel") }}
			</a>
		</div>
	</form>

	<form class="mt-10 border-t pt-6"
	      hx-delete="{{ .Prefix }}/{{ .Row.ID }}"
	      hx-confirm="{{ .Labels.T("message.deleted") }}">
		@csrf
		{!! components.Button(components.ButtonProps{
			Label:   .Labels.T("control.delete"),
			Type:    "submit",
			Variant: "destructive",
			Size:    "sm",
		}) !!}
	</form>
@endsection
