"use strict";

/**
 * Pins the search filters sidebar when the column would scroll under the navbar.
 * Uses the column's position (not the sticky element's) so we pin before the sticky "unsticks".
 */
(function () {
  const NAVBAR_CLEARANCE_PX = 96; // 6rem – keep below fixed navbar

  const sticky = document.querySelector("#search-filters-sidebar-col .search-filters-sidebar-sticky");
  if (!sticky) return;

  const column = document.getElementById("search-filters-sidebar-col");
  if (!column) return;

  let ticking = false;

  function updatePin() {
    if (window.getComputedStyle(column).display === "none") {
      sticky.classList.remove("is-pinned");
      sticky.style.top = "";
      sticky.style.left = "";
      sticky.style.width = "";
      column.style.minHeight = "";
      ticking = false;
      return;
    }
    const colRect = column.getBoundingClientRect();
    const stickyRect = sticky.getBoundingClientRect();

    // Pin when column top would go under the navbar (pin proactively)
    if (colRect.top < NAVBAR_CLEARANCE_PX) {
      if (!sticky.classList.contains("is-pinned")) {
        sticky.classList.add("is-pinned");
        column.style.minHeight = `${stickyRect.height}px`;
      }
      sticky.style.top = `${NAVBAR_CLEARANCE_PX}px`;
      sticky.style.left = `${colRect.left}px`;
      sticky.style.width = `${colRect.width}px`;
    } else {
      if (sticky.classList.contains("is-pinned")) {
        sticky.classList.remove("is-pinned");
        sticky.style.top = "";
        sticky.style.left = "";
        sticky.style.width = "";
        column.style.minHeight = "";
      }
    }
    ticking = false;
  }

  function onScrollOrResize() {
    if (ticking) return;
    ticking = true;
    requestAnimationFrame(updatePin);
  }

  window.addEventListener("scroll", onScrollOrResize, { passive: true });
  window.addEventListener("resize", onScrollOrResize);

  // Initial check (e.g. refresh at bottom of page)
  updatePin();
})();
