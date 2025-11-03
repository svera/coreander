"use strict";

// Control checkbox state when marking a document as complete/incomplete
document.body.addEventListener('htmx:afterRequest', function(evt) {
    if (!evt.detail.elt.id || !evt.detail.elt.id.startsWith("complete-checkbox-")) {
        return
    }

    // Check for 403 Forbidden (session expired)
    if (evt.detail.xhr && evt.detail.xhr.status === 403) {
        window.location.reload();
        return;
    }

    if (!evt.detail.successful) {
        // If request failed, revert the checkbox state
        const checkboxEl = evt.detail.elt;
        checkboxEl.checked = !checkboxEl.checked;
        return
    }

    const checkboxEl = evt.detail.elt;

    // Update the title based on the new state
    if (checkboxEl.checked) {
        checkboxEl.title = checkboxEl.getAttribute('data-incomplete-title');
    } else {
        checkboxEl.title = checkboxEl.getAttribute('data-complete-title');
    }

    // After htmx loads new date input, set max to today
    setTimeout(() => {
        const today = new Date().toISOString().split('T')[0];
        document.querySelectorAll('input[id^="completion-date-"]').forEach(input => {
            if (!input.hasAttribute('max')) {
                input.setAttribute('max', today);
            }
        });
    }, 0);
});

// Handle completion date changes
document.addEventListener('DOMContentLoaded', function() {
    // Set max date to today for all completion date inputs
    const today = new Date().toISOString().split('T')[0];
    document.querySelectorAll('input[id^="completion-date-"]').forEach(input => {
        input.setAttribute('max', today);
    });

    // Add hover and focus effects for date inputs
    document.body.addEventListener('mouseover', function(evt) {
        if (evt.target.id && evt.target.id.startsWith("completion-date-")) {
            evt.target.style.textDecoration = 'underline';
        }
    });

    document.body.addEventListener('mouseout', function(evt) {
        if (evt.target.id && evt.target.id.startsWith("completion-date-")) {
            evt.target.style.textDecoration = 'none';
        }
    });

    document.body.addEventListener('focus', function(evt) {
        if (evt.target.id && evt.target.id.startsWith("completion-date-")) {
            evt.target.classList.add('border', 'border-primary', 'rounded');
        }
    }, true);

    document.body.addEventListener('blur', function(evt) {
        if (evt.target.id && evt.target.id.startsWith("completion-date-")) {
            evt.target.classList.remove('border', 'border-primary', 'rounded');
        }
    }, true);

    document.body.addEventListener('change', function(evt) {
        if (!evt.target.id || !evt.target.id.startsWith("completion-date-")) {
            return;
        }

        const input = evt.target;
        const slug = input.dataset.slug;
        const newDate = input.value;
        const originalDate = input.dataset.originalDate;

        // Only update if the date actually changed
        if (newDate === originalDate) {
            return;
        }

        // Validate date format
        if (!newDate || !/^\d{4}-\d{2}-\d{2}$/.test(newDate)) {
            console.error('Invalid date format:', newDate);
            input.value = originalDate;
            return;
        }

        // Prevent future dates (compare only date parts)
        const selectedDate = new Date(newDate + 'T00:00:00');
        const today = new Date();
        today.setHours(0, 0, 0, 0);
        if (selectedDate > today) {
            console.error('Future dates not allowed:', newDate);
            input.value = originalDate;
            return;
        }

        // Send the update request
        fetch(`/documents/${slug}/complete`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({ completed_at: newDate })
        })
        .then(response => {
            if (response.ok) {
                // Update the original date to the new value
                input.dataset.originalDate = newDate;
            } else if (response.status === 403) {
                // Session expired, reload the page
                window.location.reload();
            } else {
                throw new Error('Failed to update date');
            }
        })
        .catch(error => {
            console.error('Error updating completion date:', error);
            // Revert to original date silently
            input.value = originalDate;
        });
    });
});


