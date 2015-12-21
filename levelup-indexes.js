var db = levelupIndexes.configure(db,
  [
    {
      name: 'comment',
      indexes: [
        {
          name: 'documentComment',
          fields: [
            'documentId',
            'id',
            'date'
          ]
        }
      ],
      streams: {
        {
          name: 'documentCommentEvents'
          fields: [
            'documentId',
            'id',
          ]
        }
      }

    }
  ]
);

function dbIndex(indexId) {
  return {
    createReadStream : function (params) {
      params.start == undefined ? undefined :
    }
  };
}
