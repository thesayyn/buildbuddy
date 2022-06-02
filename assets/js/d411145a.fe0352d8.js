"use strict";(self.webpackChunkbuildbuddy_docs_website=self.webpackChunkbuildbuddy_docs_website||[]).push([[2826],{4137:function(e,t,r){r.d(t,{Zo:function(){return s},kt:function(){return m}});var n=r(7294);function o(e,t,r){return t in e?Object.defineProperty(e,t,{value:r,enumerable:!0,configurable:!0,writable:!0}):e[t]=r,e}function a(e,t){var r=Object.keys(e);if(Object.getOwnPropertySymbols){var n=Object.getOwnPropertySymbols(e);t&&(n=n.filter((function(t){return Object.getOwnPropertyDescriptor(e,t).enumerable}))),r.push.apply(r,n)}return r}function i(e){for(var t=1;t<arguments.length;t++){var r=null!=arguments[t]?arguments[t]:{};t%2?a(Object(r),!0).forEach((function(t){o(e,t,r[t])})):Object.getOwnPropertyDescriptors?Object.defineProperties(e,Object.getOwnPropertyDescriptors(r)):a(Object(r)).forEach((function(t){Object.defineProperty(e,t,Object.getOwnPropertyDescriptor(r,t))}))}return e}function u(e,t){if(null==e)return{};var r,n,o=function(e,t){if(null==e)return{};var r,n,o={},a=Object.keys(e);for(n=0;n<a.length;n++)r=a[n],t.indexOf(r)>=0||(o[r]=e[r]);return o}(e,t);if(Object.getOwnPropertySymbols){var a=Object.getOwnPropertySymbols(e);for(n=0;n<a.length;n++)r=a[n],t.indexOf(r)>=0||Object.prototype.propertyIsEnumerable.call(e,r)&&(o[r]=e[r])}return o}var l=n.createContext({}),d=function(e){var t=n.useContext(l),r=t;return e&&(r="function"==typeof e?e(t):i(i({},t),e)),r},s=function(e){var t=d(e.components);return n.createElement(l.Provider,{value:t},e.children)},c={inlineCode:"code",wrapper:function(e){var t=e.children;return n.createElement(n.Fragment,{},t)}},p=n.forwardRef((function(e,t){var r=e.components,o=e.mdxType,a=e.originalType,l=e.parentName,s=u(e,["components","mdxType","originalType","parentName"]),p=d(r),m=o,b=p["".concat(l,".").concat(m)]||p[m]||c[m]||a;return r?n.createElement(b,i(i({ref:t},s),{},{components:r})):n.createElement(b,i({ref:t},s))}));function m(e,t){var r=arguments,o=t&&t.mdxType;if("string"==typeof e||o){var a=r.length,i=new Array(a);i[0]=p;var u={};for(var l in t)hasOwnProperty.call(t,l)&&(u[l]=t[l]);u.originalType=e,u.mdxType="string"==typeof e?e:o,i[1]=u;for(var d=2;d<a;d++)i[d]=r[d];return n.createElement.apply(null,i)}return n.createElement.apply(null,r)}p.displayName="MDXCreateElement"},1548:function(e,t,r){r.r(t),r.d(t,{frontMatter:function(){return u},contentTitle:function(){return l},metadata:function(){return d},assets:function(){return s},toc:function(){return c},default:function(){return m}});var n=r(7462),o=r(3366),a=(r(7294),r(4137)),i=["components"],u={slug:"buildbuddy-v1-8-0-release-notes",title:"BuildBuddy v1.8.0 Release Notes",author:"Siggi Simonarson",author_title:"Co-founder @ BuildBuddy",date:"2021-03-18:12:00:00",author_url:"https://www.linkedin.com/in/siggisim/",author_image_url:"https://avatars.githubusercontent.com/u/1704556?v=4",tags:["product","release-notes","team"]},l=void 0,d={permalink:"/blog/buildbuddy-v1-8-0-release-notes",editUrl:"https://github.com/buildbuddy-io/buildbuddy/edit/master/website/blog/buildbuddy-v1-8-0-release-notes.md",source:"@site/blog/buildbuddy-v1-8-0-release-notes.md",title:"BuildBuddy v1.8.0 Release Notes",description:"We're excited to share that v1.8.0 of BuildBuddy is live on Cloud Hosted BuildBuddy, Enterprise, and Open Source via GitHub, Docker, and our Helm Charts!",date:"2021-03-18T12:00:00.000Z",formattedDate:"March 18, 2021",tags:[{label:"product",permalink:"/blog/tags/product"},{label:"release-notes",permalink:"/blog/tags/release-notes"},{label:"team",permalink:"/blog/tags/team"}],readingTime:2.95,truncated:!0,authors:[{name:"Siggi Simonarson",title:"Co-founder @ BuildBuddy",url:"https://www.linkedin.com/in/siggisim/",imageURL:"https://avatars.githubusercontent.com/u/1704556?v=4"}],prevItem:{title:"BuildBuddy Achieves SOC 2 Certification",permalink:"/blog/buildbuddy-achieves-soc-2-certification"},nextItem:{title:"BuildBuddy v1.5.0 Release Notes",permalink:"/blog/buildbuddy-v1-5-0-release-notes"}},s={authorsImageUrls:[void 0]},c=[],p={toc:c};function m(e){var t=e.components,r=(0,o.Z)(e,i);return(0,a.kt)("wrapper",(0,n.Z)({},p,r,{components:t,mdxType:"MDXLayout"}),(0,a.kt)("p",null,"We're excited to share that v1.8.0 of BuildBuddy is live on ",(0,a.kt)("a",{parentName:"p",href:"https://app.buildbuddy.io/"},"Cloud Hosted BuildBuddy"),", Enterprise, and Open Source via ",(0,a.kt)("a",{parentName:"p",href:"https://github.com/buildbuddy-io/buildbuddy"},"GitHub"),", ",(0,a.kt)("a",{parentName:"p",href:"https://github.com/buildbuddy-io/buildbuddy/blob/master/docs/on-prem.md#docker-image"},"Docker"),", and ",(0,a.kt)("a",{parentName:"p",href:"https://github.com/buildbuddy-io/buildbuddy-helm"},"our Helm Charts"),"!"),(0,a.kt)("p",null,"Thanks to everyone using open source, cloud-hosted, and enterprise BuildBuddy. We've made lots of improvements in this release based on your feedback."),(0,a.kt)("p",null,(0,a.kt)("strong",{parentName:"p"},"A special thank you to our new open-source contributor:")),(0,a.kt)("ul",null,(0,a.kt)("li",{parentName:"ul"},(0,a.kt)("a",{parentName:"li",href:"https://github.com/ashleydavies"},(0,a.kt)("strong",{parentName:"a"},"Ashley Davies"))," who contributed several pull requests to our ",(0,a.kt)("a",{parentName:"li",href:"https://github.com/buildbuddy-io/buildbuddy-helm/"},"Helm charts")," in order to make them easier to use in clusters that already have an Nginx controller deployed.")),(0,a.kt)("p",null,(0,a.kt)("strong",{parentName:"p"},"And a warm welcome to our three new team members!")),(0,a.kt)("ul",null,(0,a.kt)("li",{parentName:"ul"},(0,a.kt)("a",{parentName:"li",href:"https://www.linkedin.com/in/pari-parajuli/"},(0,a.kt)("strong",{parentName:"a"},"Pari Parajuli"))," who joins our engineering team as an intern who's currently studying at University of California, Berkeley."),(0,a.kt)("li",{parentName:"ul"},(0,a.kt)("a",{parentName:"li",href:"https://www.linkedin.com/in/vadimberezniker/"},(0,a.kt)("strong",{parentName:"a"},"Vadim Berezniker"))," who joins our engineering team after 7 years at Google on the Google Cloud team."),(0,a.kt)("li",{parentName:"ul"},(0,a.kt)("a",{parentName:"li",href:"https://www.linkedin.com/in/zoey-greer/"},(0,a.kt)("strong",{parentName:"a"},"Zoey Greer"))," who joins us as a software engineer from the Google Search team.")),(0,a.kt)("p",null,"We're excited to continue growing BuildBuddy and fulfill our mission of making developers more productive!"),(0,a.kt)("p",null,"Our focus for this release was on reliability, performance, improved documentation, and making BuildBuddy easier to release and monitor."))}m.isMDXComponent=!0}}]);