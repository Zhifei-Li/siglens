/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

"use strict";

let svgWidth;

$(document).ready(() => {
  displayNavbar();
  $(".theme-btn").on("click", themePickerHandler);
  if (Cookies.get("theme")) {
    theme = Cookies.get("theme");
    $("body").attr("data-theme", theme);
  }
  svgWidth = $("#timeline-container").width();
  // displayTimeline();

  const traceId = getParameterFromUrl("trace_id");
  getTraceInformation(traceId);
});

function getParameterFromUrl(param) {
  const urlParams = new URLSearchParams(window.location.search);
  return urlParams.get(param);
}

function getTraceInformation(traceId) {
  console.log("traceId: " + traceId);
  $.ajax({
    method: "POST",
    url: "api/traces/ganttchart",
    headers: {
      "Content-Type": "application/json; charset=utf-8",
      Accept: "*/*",
    },
    data: JSON.stringify({
      searchText: `trace_id=${traceId}`,
      startEpoch: "now-3h",
      endEpoch: "now",
    }),
    dataType: "json",
    crossDomain: true,
  }).then(function (res) {
    displayTimeline(res);
  });
}

$(".back-to-search-traces").on("click", function () {
  window.location.href = "search-traces.html";
});

function displayTimeline(data) {
  console.log(data);
  // console.log(data)
  // const data = {
  //     name: "Trace 1",
  //     start_time: 0,
  //     end_time: 160,
  //     children: [
  //         {
  //             name: "Span 3",
  //             start_time: 110,
  //             end_time: 150,
  //             children: [
  //                 { name: "Sub-Span 3-1", start_time: 75, end_time: 85 },
  //                 { name: "Sub-Span 3-2", start_time: 85, end_time: 150 },
  //                 { name: "Sub-Span 3-3", start_time: 95, end_time: 150 },
  //             ],
  //         },
  //         {
  //             name: "Span 2",
  //             start_time: 50,
  //             end_time: 90,
  //             children: [
  //                 { name: "Sub-Span 2-2", start_time: 75, end_time: 85 },
  //                 { name: "Sub-Span 2-1", start_time: 55, end_time: 70 },
  //             ],
  //         },
  //         {
  //             name: "Span 1",
  //             start_time: 10,
  //             end_time: 40,
  //             children: [
  //                 {
  //                     name: "Sub-Span 1-2",
  //                     start_time: 30,
  //                     end_time: 35,
  //                     children: [
  //                         {
  //                             name: "Sub-Span 1-1-2",
  //                             start_time: 34,
  //                             end_time: 38,
  //                         },
  //                     ],
  //                 },
  //                 { name: "Sub-Span 1-1", start_time: 15, end_time: 25 },
  //             ],
  //         },
  //     ],
  // };

  const totalHeight = calculateTotalHeight(data);
  const padding = { top: 20, right: 20, bottom: 20, left: 20 };

  const svg = d3
    .select("#timeline-container")
    .append("svg")
    .attr("width", svgWidth + padding.left + padding.right - 50)
    .attr("height", totalHeight + padding.top + padding.bottom)
    .append("g")
    .attr("transform", "translate(" + padding.left + "," + padding.top + ")");

  const xScale = d3
    .scaleLinear()
    .domain([data.start_time, data.end_time]) // root node end_time
    .range([400, svgWidth - 100]);

  // Add a time grid
  const timeTicks = xScale.ticks(4); // number of ticks
  svg
    .selectAll(".time-tick")
    .data(timeTicks)
    .enter()
    .append("line")
    .attr("class", "time-tick")
    .attr("x1", (d) => xScale(d))
    .attr("x2", (d) => xScale(d))
    .attr("y1", 50)
    .attr("y2", 50 + totalHeight)
    .attr("stroke", "#ccc")
    .attr("stroke-width", 1)
    .style("shape-rendering", "crispEdges");

  // Add time labels
  svg
    .selectAll(".time-label")
    .data(timeTicks)
    .enter()
    .append("text")
    .attr("class", "time-label")
    .attr("x", (d) => xScale(d))
    .attr("y", 50)
    .attr("text-anchor", "middle")
    .text((d) => d)
    .style("font-size", "15px");

  // recursively render the timeline
  let y = 100; // initial y position
  function renderTimeline(node) {
    if (node.children) {
      node.children.sort((a, b) => a.start_time - b.start_time); // sort by start time
    }
    const label = svg
      .append("text")
      .attr("x", 0)
      .attr("y", y + 12)
      .text(`${node.service_name}`)
      .style("font-size", "15px");

    if (node.children) {
      label
        .append("tspan")
        .style("fill", "gray")
        .style("font-size", "10px")
        .style("margin-left", "5px");
    }
    console.log(`
        node info :
        ${node.service_name} : ${node.operation_name}
        ${xScale(node.start_time)}
        ${node.duration}
        ${xScale(node.end_time) - xScale(node.start_time)}
        `);
    const rect = svg
      .append("rect")
      .attr("x", xScale(node.start_time))
      .attr("y", y)
      .attr("width", xScale(node.end_time) - xScale(node.start_time))
      .attr("height", 20)
      .attr("fill", "steelblue")
      .on("mouseover", () => {
        rect.style("cursor", "pointer"); // Change cursor to pointer on hover
        tooltip.style("display", "block").html(
          `
                        SpanId : ${node.span_id} <br>
                        Name: ${node.service_name} : ${
            node.operation_name
          }<br>Start Time: ${node.start_time}<br>End Time: ${
            node.end_time
          }<br>Duration: ${node.end_time - node.start_time}`
        );
      })
      .on("mousemove", (event) => {
        tooltip
          .style("left", event.pageX + 10 + "px")
          .style("top", event.pageY - 28 + "px");
      })
      .on("mouseout", () => {
        rect.style("cursor", "default"); // Change cursor back to default on mouseout
        tooltip.style("display", "none");
      });

    // Increment y for the next node
    y += 50;

    // Recursively render children
    if (node.children) {
      node.children.forEach((child) => {
        renderTimeline(child);
      });
    }
  }

  const tooltip = d3
    .select("body")
    .append("div")
    .style("position", "absolute")
    .style("z-index", "10")
    .style("display", "none")
    .style("background", "#fff")
    .style("border", "1px solid #ddd")
    .style("padding", "5px")
    .style("border-radius", "3px");

  // Render the timeline starting from the root node
  renderTimeline(data);
}

function calculateTotalHeight(node) {
  let totalHeight = 0;
  function calculateHeight(node) {
    totalHeight += 50;
    if (node.children !== null) {
      node.children.forEach(calculateHeight);
    }
  }
  calculateHeight(node);
  return totalHeight + 200;
}
