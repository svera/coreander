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
        
        img.addEventListener('load', (event) => {
            if (img.getAttribute('src') == '/images/generic.jpg') {
                return;
            }
            let cardTitle = img.nextElementSibling.getElementsByClassName('card-title')[0];
            let cardTitleLink = cardTitle.getElementsByTagName('a')[0]
            if (cardTitleLink) {
                cardTitleLink.text = '';
            } else {
                cardTitle.innerHTML = ''
            }
            img.nextElementSibling.getElementsByClassName('card-text')[0].innerHTML = '';
        });
    }
}
