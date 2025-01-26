"use strict";

import { DateTime } from "./luxon.min.js";

const datetimeFormatter = () => {
    const datetime = document.querySelectorAll('.datetime span');
    datetime.forEach(function(element) {
        const dt = DateTime.fromISO(element.textContent);
        if (dt.isValid) {
            element.textContent = dt.toRelative({ locale: document.documentElement.lang });
        }
    });
}

document.addEventListener('DOMContentLoaded', datetimeFormatter());

const observer = new MutationObserver(datetimeFormatter);

// Start observing the target node for configured mutations
const node = document.getElementById("list");
observer.observe(node, { attributes: true, childList: false, subtree: true });
