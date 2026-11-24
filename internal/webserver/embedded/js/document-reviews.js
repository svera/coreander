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

function showReviewsError(event) {
    const reviews = event.detail?.target;
    if (!reviews?.matches('[data-document-reviews]')) {
        return;
    }
    const loading = reviews.querySelector('[data-reviews-initial-loading]');
    if (loading) {
        loading.hidden = true;
    }
    const error = reviews.querySelector('[data-reviews-error]');
    if (error) {
        error.hidden = false;
    }
}

document.body.addEventListener('htmx:responseError', showReviewsError);
document.body.addEventListener('htmx:sendError', showReviewsError);
document.body.addEventListener('htmx:timeout', showReviewsError);

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
