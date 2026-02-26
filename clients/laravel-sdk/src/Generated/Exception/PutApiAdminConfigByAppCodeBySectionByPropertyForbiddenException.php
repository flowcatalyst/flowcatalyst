<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminConfigByAppCodeBySectionByPropertyForbiddenException extends ForbiddenException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse403
     */
    private $apiAdminConfigAppCodeSectionPropertyPutResponse403;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse403 $apiAdminConfigAppCodeSectionPropertyPutResponse403, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminConfigAppCodeSectionPropertyPutResponse403 = $apiAdminConfigAppCodeSectionPropertyPutResponse403;
        $this->response = $response;
    }
    public function getApiAdminConfigAppCodeSectionPropertyPutResponse403(): \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse403
    {
        return $this->apiAdminConfigAppCodeSectionPropertyPutResponse403;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}