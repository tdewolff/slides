<!doctype html>
<title>{{.Title}}</title>
<link rel=stylesheet href=res/style.css>

<div id=slides>
    {{range $i, $slide := .Content}}
    <div class="slide{{if eq $i 0}} title{{end}}">
        <div class="slide-content">
            {{$slide}}
        </div>
        <div class="slide-number">{{add $i 1}}</div>
    </div>
    {{end}}
</div>

<script src=res/script.js></script>
