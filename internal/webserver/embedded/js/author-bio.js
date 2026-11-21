"use strict";

const bioObserver = new ResizeObserver(entries => {
    for (const { target: content } of entries) {
        const button = content.nextElementSibling;
        const expanded = button.getAttribute('aria-expanded') === 'true';
        button.hidden = !expanded && content.scrollHeight <= content.clientHeight;
    }
});

function observeBio() {
    bioObserver.disconnect();
    document.querySelectorAll('.author-bio-text').forEach(content => {
        bioObserver.observe(content);
    });
}

observeBio();
document.body.addEventListener('htmx:afterSwap', observeBio);

document.body.addEventListener('click', event => {
    const button = event.target.closest('.author-bio-toggle');
    if (!button) {
        return;
    }

    const content = document.getElementById(button.getAttribute('aria-controls'));
    const expanded = button.getAttribute('aria-expanded') !== 'true';
    content.classList.toggle('clamp', !expanded);
    button.setAttribute('aria-expanded', String(expanded));
    button.textContent = expanded ? button.dataset.showLess : button.dataset.showMore;
});
