window.onload = function () {
    let imgs = document.querySelectorAll('.cover');
    for (i = 0; i < imgs.length; i++) {
        let img = imgs[i];

        if (!img.getAttribute('data-src')) {
            continue;
        }

        img.addEventListener('error', function onError(e) {
            this.setAttribute('src', '/images/generic.jpg');
        });

        img.setAttribute('src', img.getAttribute('data-src'));
    }
}
