window.onload = function () {
    let imgs = document.querySelectorAll('.cover');
    for (i = 0; i < imgs.length; i++) {
        if (imgs[i].getAttribute('data-src')) {
            imgs[i].addEventListener('error', function onError(e) {
                this.setAttribute('src', '/images/generic.jpg');
            });
            imgs[i].setAttribute('src', imgs[i].getAttribute('data-src'));
            let img = imgs[i];
            imgs[i].addEventListener('load', (event) => {
                if (img.getAttribute('src') != '/images/generic.jpg') {
                    let cardTitle = img.nextElementSibling.getElementsByClassName('card-title')[0];
                    let cardTitleLink = cardTitle.getElementsByTagName('a')[0]
                    if (cardTitleLink != null) {
                        cardTitleLink.text = '';
                    } else {
                        cardTitle.innerHTML = ''
                    }
                    img.nextElementSibling.getElementsByClassName('card-text')[0].innerHTML = '';
                }
            });
        }
    }
}
