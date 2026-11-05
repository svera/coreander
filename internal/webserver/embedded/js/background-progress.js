function showBackgroundProgressError(event) {
    const progress = event.detail.elt;
    if (!(progress instanceof HTMLElement) || progress.id !== 'background-progress') {
        return;
    }

    const alert = document.createElement('div');
    alert.className = 'alert alert-warning';
    alert.setAttribute('role', 'alert');
    alert.textContent = progress.dataset.errorMessage;
    progress.replaceChildren(alert);
}

document.body.addEventListener('htmx:sendError', showBackgroundProgressError);
document.body.addEventListener('htmx:responseError', showBackgroundProgressError);
document.body.addEventListener('htmx:timeout', showBackgroundProgressError);
