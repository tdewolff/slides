let nSlides = document.querySelectorAll('.slide').length;
let currentSlide = sessionStorage.getItem('i');
if (currentSlide === null || currentSlide < 0 || currenSlide >= nSlides) {
  currentSlide = 0;
}

function updateView(d) {
  currentSlide += d;
  const marginLeft = -currentSlide * 75;
  document.getElementById('slides').style.marginLeft = marginLeft.toString() + 'vw';
}

document.onkeydown = function(e) {
  e = e || window.event;
  if (e.keyCode == 37) {
    // left
    if (currentSlide > 0) {
      updateView(-1);
    }
  } else if (e.keyCode == 39) {
    // right
    if (currentSlide < nSlides - 1) {
      updateView(1);
    }
  }
};

// Scale down when height too small
function resize() {
  const h = window.innerHeight;
  const minHeight = window.innerWidth * 0.8 * 9/16;
  if (h < minHeight) {
    const scale = h / minHeight;
    document.body.style.transform = 'scale(' + scale + ',' + scale + ')';
  } else if (document.body.style.transform != '') {
    document.body.style.transform = '';
  }
}
window.onresize = function(e) {
  resize();
};
resize();

// Scale down slide content to fit in slide
function updateSlides() {
  const slides = document.querySelectorAll('.slide')
  for (let i = 0; i < slides.length; i++) {
    const slide = slides[i];
    const slideStyle = getComputedStyle(slide);
    const maxHeight = slide.getBoundingClientRect().height - parseFloat(slideStyle.paddingTop) - parseFloat(slideStyle.paddingBottom);

    const content = slide.querySelector('.slide-content');
    const h = content.getBoundingClientRect().height;
    if (h > maxHeight) {
      const scale = maxHeight / h;
      content.style.transform = 'scale(' + scale + ',' + scale + ')'
      if (scale < 0.7) {
        console.log('Too much content in slide ' + (i+1));
      }
    }
  }
}

var ws = new WebSocket('ws://localhost:8080/ws');
ws.onopen = function(e) {
  ws.send(JSON.stringify({'Type': 'watch', 'Data': window.location.pathname}));

  updateSlides();
};

ws.onclose = function(e) {
  console.log('Connection closed');
};

ws.onerror = function(e) {
  console.log('Error:', e);
};

ws.onmessage = function(e) {
  const msg = JSON.parse(e.data);
  if (msg.Type == 'refresh') {
    window.location.reload(true);
  } else {
    console.log('Message unhandled:', e);
  }
};
