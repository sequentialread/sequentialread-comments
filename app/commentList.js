var marked = require('marked');
var fs = require('fs');
var _ = require('lodash');
var Handlebars = require('handlebars');

Handlebars.registerPartial(
  'commentList',
  fs.readFileSync('./app/commentList.html', 'utf8')
);

marked.setOptions({
  renderer: new marked.Renderer(),
  gfm: true,
  tables: true,
  breaks: false,
  pedantic: false,
  sanitize: true,
  smartLists: true,
  smartypants: false
});

module.exports = function commentList(commentKeyValues) {
  return commentKeyValues.map(keyValue => {
    var comment = _.clone(keyValue.value);

    if(!comment.username || comment.username.trim() == "") {
      comment.username = "Unknown";
    }
    comment.body = marked(comment.body);

    return comment;
  });
}
