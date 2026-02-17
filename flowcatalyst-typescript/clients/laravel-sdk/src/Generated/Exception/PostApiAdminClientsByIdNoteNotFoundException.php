<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminClientsByIdNoteNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsIdNotesPostResponse404
     */
    private $apiAdminClientsIdNotesPostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsIdNotesPostResponse404 $apiAdminClientsIdNotesPostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsIdNotesPostResponse404 = $apiAdminClientsIdNotesPostResponse404;
        $this->response = $response;
    }
    public function getApiAdminClientsIdNotesPostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminClientsIdNotesPostResponse404
    {
        return $this->apiAdminClientsIdNotesPostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}