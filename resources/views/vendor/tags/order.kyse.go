//go:build kyse

package tags

import (
	"github.com/arandu-io/kyse/components"

	tags "github.com/hyz-is/arandu-tags"
)

@go
// OrderData is what the handler hands this page.
type OrderData = tags.OrderPageData
@endgo

@extends('layouts.app')

@section('content')
	<div>
		<a class="text-muted-foreground text-sm hover:underline"
		   href="{{ .Prefix }}?type={{ .Taxonomy }}">{{ .Labels.T("control.back") }}</a>
		<h1 class="mt-2 text-2xl font-semibold tracking-tight">{{ .Labels.T("screen.order_title") }}</h1>
		<p class="text-muted-foreground mt-1 text-sm">{{ .Labels.T("screen.order_lead") }}</p>
	</div>

	{{-- The order is moved with buttons and not by dragging, and that is a
	     decision rather than an omission. A drag needs a script served beside
	     the page, and it needs a keyboard path written a second time for
	     everybody who does not use a mouse. Four buttons are the keyboard path,
	     they are what a screen reader announces, and nothing has to be
	     downloaded for them to work. --}}
	@if(len(.Rows) > 0)
		<ol class="mt-8 grid gap-2">
			@foreach(.Rows as row)
				<li class="card flex flex-wrap items-center justify-between gap-3 p-3">
					<div class="min-w-0">
						<span class="text-sm font-semibold">{{ row.Label }}</span>
						<span class="text-muted-foreground ml-2 font-mono text-xs">{{ row.Position }}</span>
					</div>
					<div class="flex items-center gap-1">
						<form hx-post="{{ .Prefix }}/{{ row.ID }}/move">
							@csrf
							<input type="hidden" name="direction" value="start">
							{!! components.Button(components.ButtonProps{
								Label:    .Labels.T("control.move_to_start"),
								Type:     "submit",
								Variant:  "ghost",
								Size:     "sm",
								Disabled: row.First,
							}) !!}
						</form>
						<form hx-post="{{ .Prefix }}/{{ row.ID }}/move">
							@csrf
							<input type="hidden" name="direction" value="up">
							{!! components.Button(components.ButtonProps{
								Label:    .Labels.T("control.move_up"),
								Type:     "submit",
								Variant:  "outline",
								Size:     "sm",
								Disabled: row.First,
							}) !!}
						</form>
						<form hx-post="{{ .Prefix }}/{{ row.ID }}/move">
							@csrf
							<input type="hidden" name="direction" value="down">
							{!! components.Button(components.ButtonProps{
								Label:    .Labels.T("control.move_down"),
								Type:     "submit",
								Variant:  "outline",
								Size:     "sm",
								Disabled: row.Last,
							}) !!}
						</form>
						<form hx-post="{{ .Prefix }}/{{ row.ID }}/move">
							@csrf
							<input type="hidden" name="direction" value="end">
							{!! components.Button(components.ButtonProps{
								Label:    .Labels.T("control.move_to_end"),
								Type:     "submit",
								Variant:  "ghost",
								Size:     "sm",
								Disabled: row.Last,
							}) !!}
						</form>
					</div>
				</li>
			@endforeach
		</ol>

		{{-- And the whole order in one write, for a screen that rearranged
		     several labels before it was ready: the identifiers go in the order
		     they are read here, and the server writes one block of positions
		     rather than one move per row. --}}
		<form class="mt-8 border-t pt-6" hx-post="{{ .Prefix }}/order">
			@csrf
			<input type="hidden" name="type" value="{{ .Taxonomy }}">
			@foreach(.Rows as row)
				<input type="hidden" name="id" value="{{ row.ID }}">
			@endforeach
			{!! components.Button(components.ButtonProps{
				Label:   .Labels.T("control.apply_order"),
				Type:    "submit",
				Variant: "outline",
			}) !!}
		</form>
	@else
		<p class="text-muted-foreground mt-8 text-sm">{{ .Labels.T("screen.order_empty") }}</p>
	@endif
@endsection
