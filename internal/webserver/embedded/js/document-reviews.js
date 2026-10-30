"use strict";

const reviewsObserver = new ResizeObserver(entries => {
    for (const { target: content } of entries) {
        const button = content.nextElementSibling;
        const expanded = button.getAttribute('aria-expanded') === 'true';
        button.hidden = !expanded && content.scrollHeight <= content.clientHeight;
    }
});

function observeReviews() {
    document.querySelectorAll('.review-content').forEach(content => {
        reviewsObserver.observe(content);
    });
}

observeReviews();
document.body.addEventListener('htmx:afterSwap', observeReviews);

document.body.addEventListener('click', event => {
    const button = event.target.closest('.review-toggle');
    if (!button) {
        return;
    }

    const content = document.getElementById(button.getAttribute('aria-controls'));
    const expanded = button.getAttribute('aria-expanded') !== 'true';
    content.classList.toggle('clamp', !expanded);
    button.setAttribute('aria-expanded', String(expanded));
    button.textContent = expanded ? button.dataset.showLess : button.dataset.showMore;
});
