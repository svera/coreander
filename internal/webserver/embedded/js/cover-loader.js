window.onload = function () {
    let imgs = document.querySelectorAll('.cover');
    for (i = 0; i < imgs.length; i++) {
        if (imgs[i].getAttribute('data-src')) {
            imgs[i].addEventListener('error', function onError(e) {
                this.setAttribute('src', '/images/generic.jpg');
            });
            imgs[i].setAttribute('src', imgs[i].getAttribute('data-src'));
        }
    }
}
