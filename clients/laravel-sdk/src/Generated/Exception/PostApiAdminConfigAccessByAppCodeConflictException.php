<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminConfigAccessByAppCodeConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodePostResponse409
     */
    private $apiAdminConfigAccessAppCodePostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodePostResponse409 $apiAdminConfigAccessAppCodePostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminConfigAccessAppCodePostResponse409 = $apiAdminConfigAccessAppCodePostResponse409;
        $this->response = $response;
    }
    public function getApiAdminConfigAccessAppCodePostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodePostResponse409
    {
        return $this->apiAdminConfigAccessAppCodePostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}